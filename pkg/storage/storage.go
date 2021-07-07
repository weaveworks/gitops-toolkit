package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/filter"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	patchutil "github.com/weaveworks/libgitops/pkg/util/patch"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// ErrAmbiguousFind is returned when the user requested one object from a List+Filter process.
	ErrAmbiguousFind = errors.New("two or more results were aquired when one was expected")
	// ErrNotFound is returned when the requested resource wasn't found.
	ErrNotFound = errors.New("resource not found")
	// ErrAlreadyExists is returned when when WriteStorage.Create is called for an already stored object.
	ErrAlreadyExists = errors.New("resource already exists")
)

type ReadStorage interface {
	// Get returns a new Object for the resource at the specified kind/uid path, based on the file content.
	// If the resource referred to by the given ObjectKey does not exist, Get returns ErrNotFound.
	Get(key ObjectKey) (runtime.Object, error)

	// List lists Objects for the specific kind. Optionally, filters can be applied (see the filter package
	// for more information, e.g. filter.NameFilter{} and filter.UIDFilter{})
	List(kind KindKey, opts ...filter.ListOption) ([]runtime.Object, error)

	// Find does a List underneath, also using filters, but always returns one object. If the List
	// underneath returned two or more results, ErrAmbiguousFind is returned. If no match was found,
	// ErrNotFound is returned.
	Find(kind KindKey, opts ...filter.ListOption) (runtime.Object, error)

	//
	// Partial object getters.
	// TODO: Figure out what we should do with these, do we need them and if so where?
	//

	// GetMeta returns a new Object's APIType representation for the resource at the specified kind/uid path.
	// If the resource referred to by the given ObjectKey does not exist, GetMeta returns ErrNotFound.
	GetMeta(key ObjectKey) (runtime.PartialObject, error)
	// ListMeta lists all Objects' APIType representation. In other words,
	// only metadata about each Object is unmarshalled (uid/name/kind/apiVersion).
	// This allows for faster runs (no need to unmarshal "the world"), and less
	// resource usage, when only metadata is unmarshalled into memory
	ListMeta(kind KindKey) ([]runtime.PartialObject, error)

	//
	// Cache-related methods.
	//

	// Checksum returns a string representing the state of an Object on disk
	// The checksum should change if any modifications have been made to the
	// Object on disk, it can be e.g. the Object's modification timestamp or
	// calculated checksum. If the Object is not found, ErrNotFound is returned.
	Checksum(key ObjectKey) (string, error)
	// Count returns the amount of available Objects of a specific kind
	// This is used by Caches to check if all Objects are cached to perform a List
	Count(kind KindKey) (uint64, error)

	//
	// Access to underlying Resources.
	//

	// RawStorage returns the RawStorage instance backing this Storage
	RawStorage() RawStorage
	// Serializer returns the serializer
	Serializer() serializer.Serializer

	//
	// Misc methods.
	//

	// ObjectKeyFor returns the ObjectKey for the given object
	ObjectKeyFor(obj runtime.Object) (ObjectKey, error)
	// Close closes all underlying resources (e.g. goroutines) used; before the application exits
	Close() error
}

type WriteStorage interface {
	// Create creates an entry for and stores the given Object in the storage. The Object must be new to the storage.
	// The ObjectMeta.CreationTimestamp field is set automatically to the current time if it is unset.
	Create(obj runtime.Object) error
	// Update updates the state of the given Object in the storage. The Object must exist in the storage.
	// The ObjectMeta.CreationTimestamp field is set automatically to the current time if it is unset.
	Update(obj runtime.Object) error

	// Patch performs a strategic merge patch on the Object with the given UID, using the byte-encoded patch given
	Patch(key ObjectKey, patch []byte) error
	// Delete removes an Object from the storage
	Delete(key ObjectKey) error
}

// Storage is an interface for persisting and retrieving API objects to/from a backend
// One Storage instance handles all different Kinds of Objects
type Storage interface {
	ReadStorage
	WriteStorage
}

// NewGenericStorage constructs a new Storage
func NewGenericStorage(rawStorage RawStorage, serializer serializer.Serializer, identifiers []runtime.IdentifierFactory) Storage {
	return &GenericStorage{rawStorage, serializer, patchutil.NewPatcher(serializer), identifiers}
}

// GenericStorage implements the Storage interface
type GenericStorage struct {
	raw         RawStorage
	serializer  serializer.Serializer
	patcher     patchutil.Patcher
	identifiers []runtime.IdentifierFactory
}

var _ Storage = &GenericStorage{}

func (s *GenericStorage) Serializer() serializer.Serializer {
	return s.serializer
}

// Get returns a new Object for the resource at the specified kind/uid path, based on the file content
func (s *GenericStorage) Get(key ObjectKey) (runtime.Object, error) {
	content, err := s.raw.Read(key)
	if err != nil {
		return nil, err
	}

	return s.decode(key, content)
}

// TODO: Verify this works
// GetMeta returns a new Object's APIType representation for the resource at the specified kind/uid path
func (s *GenericStorage) GetMeta(key ObjectKey) (runtime.PartialObject, error) {
	content, err := s.raw.Read(key)
	if err != nil {
		return nil, err
	}

	return s.decodeMeta(key, content)
}

// TODO: Make sure we don't save a partial object
func (s *GenericStorage) write(key ObjectKey, obj runtime.Object) error {
	// Set the content type based on the format given by the RawStorage, but default to JSON
	contentType := frame.ContentTypeJSON
	if ct := s.raw.ContentType(key); len(ct) != 0 {
		contentType = ct
	}

	// Set creationTimestamp if not already populated
	t := obj.GetCreationTimestamp()
	if t.IsZero() {
		obj.SetCreationTimestamp(metav1.Now())
	}

	var objBytes bytes.Buffer
	err := s.serializer.Encoder().Encode(frame.NewWriter(contentType, &objBytes), obj)
	if err != nil {
		return err
	}

	return s.raw.Write(key, objBytes.Bytes())
}

func (s *GenericStorage) Create(obj runtime.Object) error {
	key, err := s.ObjectKeyFor(obj)
	if err != nil {
		return err
	}

	if s.raw.Exists(key) {
		return ErrAlreadyExists
	}

	// The object was not found so we can safely create it
	return s.write(key, obj)
}

func (s *GenericStorage) Update(obj runtime.Object) error {
	key, err := s.ObjectKeyFor(obj)
	if err != nil {
		return err
	}

	if !s.raw.Exists(key) {
		return ErrNotFound
	}

	// The object was found so we can safely update it
	return s.write(key, obj)
}

// Patch performs a strategic merge patch on the object with the given UID, using the byte-encoded patch given
func (s *GenericStorage) Patch(key ObjectKey, patch []byte) error {
	oldContent, err := s.raw.Read(key)
	if err != nil {
		return err
	}

	newContent, err := s.patcher.Apply(oldContent, patch, key.GetGVK())
	if err != nil {
		return err
	}

	return s.raw.Write(key, newContent)
}

// Delete removes an Object from the storage
func (s *GenericStorage) Delete(key ObjectKey) error {
	return s.raw.Delete(key)
}

// Checksum returns a string representing the state of an Object on disk
func (s *GenericStorage) Checksum(key ObjectKey) (string, error) {
	return s.raw.Checksum(key)
}

func (s *GenericStorage) list(kind KindKey) (result []runtime.Object, walkerr error) {
	walkerr = s.walkKind(kind, func(key ObjectKey, content []byte) error {
		obj, err := s.decode(key, content)
		if err != nil {
			return err
		}

		result = append(result, obj)
		return nil
	})
	return
}

// List lists Objects for the specific kind. Optionally, filters can be applied (see the filter package
// for more information, e.g. filter.NameFilter{} and filter.UIDFilter{})
func (s *GenericStorage) List(kind KindKey, opts ...filter.ListOption) ([]runtime.Object, error) {
	// First, complete the options struct
	o, err := filter.MakeListOptions(opts...)
	if err != nil {
		return nil, err
	}

	// Do an internal list to get all objects
	objs, err := s.list(kind)
	if err != nil {
		return nil, err
	}

	// For all list filters, pipe the output of the previous as the input to the next, in order.
	for _, filter := range o.Filters {
		objs, err = filter.Filter(objs...)
		if err != nil {
			return nil, err
		}
	}
	return objs, nil
}

// Find does a List underneath, also using filters, but always returns one object. If the List
// underneath returned two or more results, ErrAmbiguousFind is returned. If no match was found,
// ErrNotFound is returned.
func (s *GenericStorage) Find(kind KindKey, opts ...filter.ListOption) (runtime.Object, error) {
	// Do a normal list underneath
	objs, err := s.List(kind, opts...)
	if err != nil {
		return nil, err
	}
	// Return based on the object count
	switch l := len(objs); l {
	case 0:
		return nil, fmt.Errorf("no Find match found: %w", ErrNotFound)
	case 1:
		return objs[0], nil
	default:
		return nil, fmt.Errorf("too many (%d) matches: %v: %w", l, objs, ErrAmbiguousFind)
	}
}

// ListMeta lists all Objects' APIType representation. In other words,
// only metadata about each Object is unmarshalled (uid/name/kind/apiVersion).
// This allows for faster runs (no need to unmarshal "the world"), and less
// resource usage, when only metadata is unmarshalled into memory
func (s *GenericStorage) ListMeta(kind KindKey) (result []runtime.PartialObject, walkerr error) {
	walkerr = s.walkKind(kind, func(key ObjectKey, content []byte) error {

		obj, err := s.decodeMeta(key, content)
		if err != nil {
			return err
		}

		result = append(result, obj)
		return nil
	})
	return
}

// Count counts the Objects for the specific kind
func (s *GenericStorage) Count(kind KindKey) (uint64, error) {
	entries, err := s.raw.List(kind)
	return uint64(len(entries)), err
}

func (s *GenericStorage) ObjectKeyFor(obj runtime.Object) (ObjectKey, error) {
	var gvk schema.GroupVersionKind
	var err error

	_, isPartialObject := obj.(runtime.PartialObject)
	if isPartialObject {
		gvk = obj.GetObjectKind().GroupVersionKind()
		// TODO: Error if empty
	} else {
		gvk, err = serializer.GVKForObject(s.serializer.Scheme(), obj)
		if err != nil {
			return nil, err
		}
	}

	id := s.identify(obj)
	if id == nil {
		return nil, fmt.Errorf("couldn't identify object")
	}
	return NewObjectKey(NewKindKey(gvk), id), nil
}

// RawStorage returns the RawStorage instance backing this Storage
func (s *GenericStorage) RawStorage() RawStorage {
	return s.raw
}

// Close closes all underlying resources (e.g. goroutines) used; before the application exits
func (s *GenericStorage) Close() error {
	return nil // nothing to do here for GenericStorage
}

// identify loops through the identifiers, in priority order, to identify the object correctly
func (s *GenericStorage) identify(obj runtime.Object) runtime.Identifyable {
	for _, identifier := range s.identifiers {

		id, ok := identifier.Identify(obj)
		if ok {
			return id
		}
	}
	return nil
}

func (s *GenericStorage) decode(key ObjectKey, content []byte) (runtime.Object, error) {
	gvk := key.GetGVK()
	// Decode the bytes to the internal version of the Object, if desired
	isInternal := gvk.Version == kruntime.APIVersionInternal

	// Decode the bytes into an Object
	ct := s.raw.ContentType(key)
	logrus.Infof("Decoding with content type %s", ct)
	obj, err := s.serializer.Decoder(
		serializer.WithConvertToHubDecode(isInternal),
	).Decode(frame.NewReader(ct, frame.FromBytes(content)))
	if err != nil {
		return nil, err
	}

	// Cast to runtime.Object, and make sure it works
	metaObj, ok := obj.(runtime.Object)
	if !ok {
		return nil, fmt.Errorf("can't convert to libgitops.runtime.Object")
	}

	// Set the desired gvk of this Object from the caller
	metaObj.GetObjectKind().SetGroupVersionKind(gvk)
	return metaObj, nil
}

func (s *GenericStorage) decodeMeta(key ObjectKey, content []byte) (runtime.PartialObject, error) {
	gvk := key.GetGVK()
	partobjs, err := DecodePartialObjects(io.NopCloser(frame.FromBytes(content)), s.serializer.Scheme(), false, &gvk)
	if err != nil {
		return nil, err
	}

	return partobjs[0], nil
}

func (s *GenericStorage) walkKind(kind KindKey, fn func(key ObjectKey, content []byte) error) error {
	keys, err := s.raw.List(kind)
	if err != nil {
		return err
	}

	for _, key := range keys {
		// Allow metadata.json to not exist, although the directory does exist
		if !s.raw.Exists(key) {
			continue
		}

		content, err := s.raw.Read(key)
		if err != nil {
			return err
		}

		if err := fn(key, content); err != nil {
			return err
		}
	}

	return nil
}

// DecodePartialObjects reads any set of frames from the given ReadCloser, decodes the frames into
// PartialObjects, validates that the decoded objects are known to the scheme, and optionally sets a default
// group
func DecodePartialObjects(rc io.ReadCloser, scheme *kruntime.Scheme, allowMultiple bool, defaultGVK *schema.GroupVersionKind) ([]runtime.PartialObject, error) {
	fr := frame.NewYAMLReader(rc)

	ctx := context.TODO()
	frames, err := frame.ReadFrameList(ctx, fr)
	if err != nil {
		return nil, err
	}

	// If we only allow one frame, signal that early
	if !allowMultiple && len(frames) != 1 {
		return nil, fmt.Errorf("DecodePartialObjects: unexpected number of frames received from ReadCloser: %d expected 1", len(frames))
	}

	objs := make([]runtime.PartialObject, 0, len(frames))
	for _, frame := range frames {
		partobj, err := runtime.NewPartialObject(frame)
		if err != nil {
			return nil, err
		}

		gvk := partobj.GetObjectKind().GroupVersionKind()

		// Don't decode API objects unknown to the scheme (e.g. Kubernetes manifests)
		if !scheme.Recognizes(gvk) {
			// TODO: Typed error
			return nil, fmt.Errorf("unknown GroupVersionKind: %s", partobj.GetObjectKind().GroupVersionKind())
		}

		if defaultGVK != nil {
			// Set the desired gvk from the caller of this Object, if defaultGVK is set
			// In practice, this means, although we got an external type,
			// we might want internal Objects later in the client. Hence,
			// set the right expectation here
			partobj.GetObjectKind().SetGroupVersionKind(gvk)
		}

		objs = append(objs, partobj)
	}
	return objs, nil
}
