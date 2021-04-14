package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrCannotSaveMetadata is returned if the user tries to save metadata-only objects
	ErrCannotSaveMetadata = errors.New("cannot save (Create|Update|Patch) *metav1.PartialObjectMetadata")
	// ErrNameRequired is returned when .metadata.name is unset
	// TODO: Support generateName?
	ErrNameRequired = errors.New(".metadata.name is required")
)

// TODO: Make a *core.Unknown that has
// 1. TypeMeta
// 2. DeepCopies (for Object compatibility),
// 3. ObjectMeta
// 4. Spec { Data []byte, ContentType ContentType, Object interface{} }
// 5. Status { Data []byte, ContentType ContentType, Object interface{} }
// TODO: Need to make sure we never write this internal struct to disk (MarshalJSON error?)

// Create an alias for the Object type
type Object = client.Object

type Accessors interface {
	Storage() storage.Storage
	NamespaceEnforcer() NamespaceEnforcer
	Encoder() serializer.Encoder
	Decoder() serializer.Decoder
}

type WriteAccessors interface {
	Validator() Validator
	StorageVersioner() StorageVersioner
}

type Reader interface {
	Accessors

	Get(ctx context.Context, obj Object) error
	storage.Lister
}

type Writer interface {
	Accessors
	WriteAccessors

	Create(ctx context.Context, obj Object) error
	Update(ctx context.Context, obj Object) error
	Delete(ctx context.Context, obj Object) error
}

type StatusWriter interface {
	Accessors
	WriteAccessors

	UpdateStatus(ctx context.Context, obj Object) error
}

// Backend combines the Reader and Writer interfaces for a fully-functioning backend
// implementation; used by the Client interface. Backend can be through as the "API Server"
// logic in between a "frontend" Client and "document" Storage. In other words, the backend
// handles serialization, versioning, validation, and policy enforcement.
//
// Any callable function should immediately abort if the given context from the client
// has expired; so an invalid context doesn't "leak down" to the Storage system.
type Backend interface {
	Reader
	Writer
	StatusWriter
}

type ChangeOperation string

const (
	ChangeOperationCreate ChangeOperation = "create"
	ChangeOperationUpdate ChangeOperation = "update"
	ChangeOperationDelete ChangeOperation = "delete"
)

type Validator interface {
	ValidateChange(ctx context.Context, backend Reader, op ChangeOperation, obj Object) error
}

// NewGeneric creates a new generic Backend for the given underlying Storage for storing the
// objects once serialized, encoders and decoders for (de)serialization, the NamespaceEnforcer
// for enforcing a namespacing policy, the StorageVersioner for telling the encoder what version
// of many to use when encoding, and optionally, a Validator.
//
// All parameters except the validator are mandatory.
func NewGeneric(
	storage storage.Storage,
	encoder serializer.Encoder,
	decoder serializer.Decoder,
	enforcer NamespaceEnforcer,
	versioner StorageVersioner,
	validator Validator,
) (*Generic, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage is mandatory")
	}
	if encoder == nil {
		return nil, fmt.Errorf("encoder is mandatory")
	}
	if decoder == nil {
		return nil, fmt.Errorf("decoder is mandatory")
	}
	if enforcer == nil {
		return nil, fmt.Errorf("enforcer is mandatory")
	}
	if versioner == nil {
		return nil, fmt.Errorf("versioner is mandatory")
	}
	return &Generic{
		// It shouldn't matter if we use the encoder's or decoder's SchemeLock
		LockedScheme: encoder.GetLockedScheme(),
		encoder:      encoder,
		decoder:      decoder,

		storage:   storage,
		enforcer:  enforcer,
		validator: validator,
		versioner: versioner,
	}, nil
}

var _ Backend = &Generic{}

type Generic struct {
	serializer.LockedScheme
	encoder serializer.Encoder
	decoder serializer.Decoder

	storage   storage.Storage
	enforcer  NamespaceEnforcer
	validator Validator
	versioner StorageVersioner
}

func (b *Generic) Encoder() serializer.Encoder {
	return b.encoder
}

func (b *Generic) Decoder() serializer.Decoder {
	return b.decoder
}

func (b *Generic) Storage() storage.Storage {
	return b.storage
}

func (b *Generic) NamespaceEnforcer() NamespaceEnforcer {
	return b.enforcer
}

func (b *Generic) Validator() Validator {
	return b.validator
}

func (b *Generic) StorageVersioner() StorageVersioner {
	return b.versioner
}

func (b *Generic) Get(ctx context.Context, obj Object) error {
	// If the context has been cancelled or timed out; directly return an error
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get the versioned ID for the given obj. This might mutate obj wrt namespacing info.
	id, err := b.idForObj(ctx, obj)
	if err != nil {
		return err
	}
	// Read the underlying bytes
	content, err := b.storage.Read(ctx, id)
	if err != nil {
		return err
	}
	// Get the right content type for the data
	ct, err := b.storage.ContentType(ctx, id)
	if err != nil {
		return err
	}

	// TODO: Check if the decoder "replaces" already-set fields or "leaks" old data?
	return b.decoder.DecodeInto(serializer.NewSingleFrameReader(content, ct), obj)
}

// ListGroupKinds returns all known GroupKinds by the implementation at that
// time. The set might vary over time as data is created and deleted; and
// should not be treated as an universal "what types could possibly exist",
// but more generally, "what are the GroupKinds of the objects that currently
// exist"? However, obviously, specific implementations might honor this
// guideline differently. This might be used for introspection into the system.
func (b *Generic) ListGroupKinds(ctx context.Context) ([]core.GroupKind, error) {
	// If the context has been cancelled or timed out; directly return an error
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return b.storage.ListGroupKinds(ctx)
}

// ListNamespaces lists the available namespaces for the given GroupKind.
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects; for that the behavior is undefined (but returning an error
// is recommended).
func (b *Generic) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	// If the context has been cancelled or timed out; directly return an error
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return b.storage.ListNamespaces(ctx, gk)
}

// ListObjectKeys returns a list of names (with optionally, the namespace).
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object keys for that given namespace.
func (b *Generic) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) (core.UnversionedObjectIDSet, error) {
	// If the context has been cancelled or timed out; directly return an error
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return b.storage.ListObjectIDs(ctx, gk, namespace)
}

func (b *Generic) Create(ctx context.Context, obj Object) error {
	// If the context has been cancelled or timed out; directly return an error
	if err := ctx.Err(); err != nil {
		return err
	}

	// We must never save metadata-only structs
	if serializer.IsPartialObject(obj) {
		return ErrCannotSaveMetadata
	}

	// Get the versioned ID for the given obj. This might mutate obj wrt namespacing info.
	id, err := b.idForObj(ctx, obj)
	if err != nil {
		return err
	}

	// Do not create it if it already exists
	if b.storage.Exists(ctx, id) {
		return core.NewErrAlreadyExists(id)
	}

	// Validate that the change is ok
	// TODO: Don't make "upcasting" possible here
	if b.validator != nil {
		if err := b.validator.ValidateChange(ctx, b, ChangeOperationCreate, obj); err != nil {
			return err
		}
	}

	// Internal, common write shared with Update()
	return b.write(ctx, id, obj)
}
func (b *Generic) Update(ctx context.Context, obj Object) error { // If the context has been cancelled or timed out; directly return an error
	// If the context has been cancelled or timed out; directly return an error
	if err := ctx.Err(); err != nil {
		return err
	}

	// We must never save metadata-only structs
	if serializer.IsPartialObject(obj) {
		return ErrCannotSaveMetadata
	}

	// Get the versioned ID for the given obj. This might mutate obj wrt namespacing info.
	id, err := b.idForObj(ctx, obj)
	if err != nil {
		return err
	}

	// Require that the object already exists
	if !b.storage.Exists(ctx, id) {
		return core.NewErrNotFound(id)
	}

	// Validate that the change is ok
	// TODO: Don't make "upcasting" possible here
	if b.validator != nil {
		if err := b.validator.ValidateChange(ctx, b, ChangeOperationUpdate, obj); err != nil {
			return err
		}
	}

	// Internal, common write shared with Create()
	return b.write(ctx, id, obj)
}

func (b *Generic) UpdateStatus(ctx context.Context, obj Object) error {
	// If the context has been cancelled or timed out; directly return an error
	if err := ctx.Err(); err != nil {
		return err
	}

	return core.ErrNotImplemented // TODO
}

func (b *Generic) write(ctx context.Context, id core.ObjectID, obj Object) error {
	// Get the content type of the object
	ct, err := b.storage.ContentType(ctx, id)
	if err != nil {
		return err
	}
	// Resolve the desired storage version
	gv, err := b.versioner.StorageVersion(id)
	if err != nil {
		return err
	}

	// Set creationTimestamp if not already populated
	t := obj.GetCreationTimestamp()
	if t.IsZero() {
		obj.SetCreationTimestamp(metav1.Now())
	}

	var objBytes bytes.Buffer
	// This FrameWriter works for any content type; and transparently writes to objBytes
	fw := serializer.NewSingleFrameWriter(&objBytes, ct)
	// The encoder is set to use the given ContentType through fw; and encodes obj.
	if err := b.encoder.EncodeForGroupVersion(fw, obj, gv); err != nil {
		return err
	}

	return b.storage.Write(ctx, id, objBytes.Bytes())
}

func (b *Generic) Delete(ctx context.Context, obj Object) error {
	// If the context has been cancelled or timed out; directly return an error
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get the versioned ID for the given obj. This might mutate obj wrt namespacing info.
	id, err := b.idForObj(ctx, obj)
	if err != nil {
		return err
	}

	// Verify it did exist
	if !b.storage.Exists(ctx, id) {
		return core.NewErrNotFound(id)
	}

	// Validate that the change is ok
	// TODO: Don't make "upcasting" possible here
	if b.validator != nil {
		if err := b.validator.ValidateChange(ctx, b, ChangeOperationDelete, obj); err != nil {
			return err
		}
	}

	// Delete it from the underlying storage
	return b.storage.Delete(ctx, id)
}

// Note: This should also work for unstructured and partial metadata objects
func (b *Generic) idForObj(ctx context.Context, obj Object) (core.ObjectID, error) {
	// Get the GroupVersionKind of the given object.
	gvk, err := serializer.GVKForObject(b.Scheme(), obj)
	if err != nil {
		return nil, err
	}

	// Object must always have .metadata.name set
	if len(obj.GetName()) == 0 {
		return nil, ErrNameRequired
	}

	// Enforce the given namespace policy. This might mutate obj.
	// TODO: disallow "upcasting" the Lister to a full-blown Storage?
	if err := b.enforcer.EnforceNamespace(
		ctx,
		obj,
		gvk,
		b.Storage().Namespacer(),
		b.Storage(),
	); err != nil {
		return nil, err
	}

	// At this point we know name is non-empty, and the namespace field is correct,
	// according to policy
	return core.NewObjectID(gvk, core.ObjectKeyFromMetav1Object(obj)), nil
}
