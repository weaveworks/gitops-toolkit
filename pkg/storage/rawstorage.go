package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	"github.com/weaveworks/libgitops/pkg/util"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RawStorage is a Key-indexed low-level interface to
// store byte-encoded Objects (resources) in non-volatile
// memory.
type RawStorage interface {
	// Read returns a resource's content based on key.
	// If the resource does not exist, it returns ErrNotFound.
	Read(key ObjectKey) ([]byte, error)
	// Exists checks if the resource indicated by key exists.
	Exists(key ObjectKey) bool
	// Write writes the given content to the resource indicated by key.
	// Error returns are implementation-specific.
	Write(key ObjectKey, content []byte) error
	// Delete deletes the resource indicated by key.
	// If the resource does not exist, it returns ErrNotFound.
	Delete(key ObjectKey) error
	// List returns all matching object keys based on the given KindKey.
	List(key KindKey) ([]ObjectKey, error)
	// Checksum returns a string checksum for the resource indicated by key.
	// If the resource does not exist, it returns ErrNotFound.
	Checksum(key ObjectKey) (string, error)
	// ContentType returns the content type of the contents of the resource indicated by key.
	ContentType(key ObjectKey) frame.ContentType

	// WatchDir returns the path for Watchers to watch changes in.
	WatchDir() string
	// GetKey retrieves the Key containing the virtual path based
	// on the given physical file path returned by a Watcher.
	GetKey(path string) (ObjectKey, error)
}

func NewGenericRawStorage(dir string, gv schema.GroupVersion, ct frame.ContentType) RawStorage {
	ext := extForContentType(ct)
	if ext == "" {
		panic("Invalid content type")
	}
	return &GenericRawStorage{
		dir: dir,
		gv:  gv,
		ct:  ct,
		ext: ext,
	}
}

// GenericRawStorage is a rawstorage which stores objects as JSON files on disk,
// in the form: <dir>/<kind>/<identifier>/metadata.json.
// The GenericRawStorage only supports one GroupVersion at a time, and will error if given
// any other resources
type GenericRawStorage struct {
	dir string
	gv  schema.GroupVersion
	ct  frame.ContentType
	ext string
}

func (r *GenericRawStorage) keyPath(key ObjectKey) string {
	return path.Join(r.dir, key.GetKind(), key.GetIdentifier(), fmt.Sprintf("metadata%s", r.ext))
}

func (r *GenericRawStorage) kindKeyPath(kindKey KindKey) string {
	return path.Join(r.dir, kindKey.GetKind())
}

func (r *GenericRawStorage) validateGroupVersion(kind KindKey) error {
	if r.gv.Group == kind.GetGroup() && r.gv.Version == kind.GetVersion() {
		return nil
	}

	return fmt.Errorf("GroupVersion %s/%s not supported by this GenericRawStorage", kind.GetGroup(), kind.GetVersion())
}

func (r *GenericRawStorage) Read(key ObjectKey) ([]byte, error) {
	// Validate GroupVersion first
	if err := r.validateGroupVersion(key); err != nil {
		return nil, err
	}

	// Check if the resource indicated by key exists
	if !r.Exists(key) {
		return nil, ErrNotFound
	}

	return ioutil.ReadFile(r.keyPath(key))
}

func (r *GenericRawStorage) Exists(key ObjectKey) bool {
	// Validate GroupVersion first
	if err := r.validateGroupVersion(key); err != nil {
		return false
	}

	return util.FileExists(r.keyPath(key))
}

func (r *GenericRawStorage) Write(key ObjectKey, content []byte) error {
	// Validate GroupVersion first
	if err := r.validateGroupVersion(key); err != nil {
		return err
	}

	file := r.keyPath(key)

	// Create the underlying directories if they do not exist already
	if !r.Exists(key) {
		if err := os.MkdirAll(path.Dir(file), 0755); err != nil {
			return err
		}
	}

	return ioutil.WriteFile(file, content, 0644)
}

func (r *GenericRawStorage) Delete(key ObjectKey) error {
	// Validate GroupVersion first
	if err := r.validateGroupVersion(key); err != nil {
		return err
	}

	// Check if the resource indicated by key exists
	if !r.Exists(key) {
		return ErrNotFound
	}

	return os.RemoveAll(path.Dir(r.keyPath(key)))
}

func (r *GenericRawStorage) List(kind KindKey) ([]ObjectKey, error) {
	// Validate GroupVersion first
	if err := r.validateGroupVersion(kind); err != nil {
		return nil, err
	}

	entries, err := ioutil.ReadDir(r.kindKeyPath(kind))
	if err != nil {
		return nil, err
	}

	result := make([]ObjectKey, 0, len(entries))
	for _, entry := range entries {
		result = append(result, NewObjectKey(kind, runtime.NewIdentifier(entry.Name())))
	}

	return result, nil
}

// This returns the modification time as a UnixNano string
// If the file doesn't exist, return ErrNotFound
func (r *GenericRawStorage) Checksum(key ObjectKey) (string, error) {
	// Validate GroupVersion first
	if err := r.validateGroupVersion(key); err != nil {
		return "", err
	}

	// Check if the resource indicated by key exists
	if !r.Exists(key) {
		return "", ErrNotFound
	}

	return checksumFromModTime(r.keyPath(key))
}

func (r *GenericRawStorage) ContentType(_ ObjectKey) frame.ContentType {
	return r.ct
}

func (r *GenericRawStorage) WatchDir() string {
	return r.dir
}

func (r *GenericRawStorage) GetKey(p string) (ObjectKey, error) {
	splitDir := strings.Split(filepath.Clean(r.dir), string(os.PathSeparator))
	splitPath := strings.Split(filepath.Clean(p), string(os.PathSeparator))

	if len(splitPath) < len(splitDir)+2 {
		return nil, fmt.Errorf("path not long enough: %s", p)
	}

	for i := 0; i < len(splitDir); i++ {
		if splitDir[i] != splitPath[i] {
			return nil, fmt.Errorf("path has wrong base: %s", p)
		}
	}
	kind := splitPath[len(splitDir)]
	uid := splitPath[len(splitDir)+1]
	gvk := schema.GroupVersionKind{
		Group:   r.gv.Group,
		Version: r.gv.Version,
		Kind:    kind,
	}

	return NewObjectKey(NewKindKey(gvk), runtime.NewIdentifier(uid)), nil
}

func checksumFromModTime(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(fi.ModTime().UnixNano(), 10), nil
}
