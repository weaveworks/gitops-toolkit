package raw

import (
	"context"
	"errors"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// ErrNamespacedMismatch is returned by Storage methods if the given UnversionedObjectID
	// carries invalid data, according to the Namespacer.
	ErrNamespacedMismatch = errors.New("mismatch between namespacing info for object and the given parameter")
)

// Storage is a Key-indexed low-level interface to
// store byte-encoded Objects (resources) in non-volatile
// memory.
//
// This Storage operates entirely on GroupKinds; without enforcing
// a specific version of the encoded data format. This is possible
// with the assumption that any older format stored at disk can be
// read successfully and converted into a more recent version.
//
// TODO: Add thread-safety so it is not possible to issue a Write() or Delete()
// at the same time as any other read operation.
type Storage interface {
	Reader
	Writer
}

// StorageCommon is an interface that contains the resources both needed
// by Reader and Writer.
type StorageCommon interface {
	// Namespacer gives access to the namespacer that is used
	Namespacer() core.Namespacer
	// Exists checks if the resource indicated by the ID exists.
	Exists(ctx context.Context, id core.UnversionedObjectID) bool
}

// Reader provides the read operations for the Storage.
type Reader interface {
	StorageCommon

	// Read returns a resource's content based on the ID.
	// If the resource does not exist, it returns core.NewErrNotFound.
	Read(ctx context.Context, id core.UnversionedObjectID) ([]byte, error)
	// Stat returns information about the object, e.g. checksum,
	// content type, and possibly, path on disk (in the case of
	// FilesystemStorage), or core.NewErrNotFound if not found
	Stat(ctx context.Context, id core.UnversionedObjectID) (ObjectInfo, error)
	// Resolve ContentType
	ContentTypeResolver

	// List operations
	Lister
}

type ContentTypeResolver interface {
	// ContentType returns the content type that should be used when serializing
	// the object with the given ID. This operation must function also before the
	// Object with the given id exists in the system, in order to be able to
	// create new Objects.
	ContentType(ctx context.Context, id core.UnversionedObjectID) (serializer.ContentType, error)
}

type Lister interface {
	// ListNamespaces lists the available namespaces for the given GroupKind.
	// This function shall only be called for namespaced objects, it is up to
	// the caller to make sure they do not call this method for root-spaced
	// objects. If any of the given rules are violated, ErrNamespacedMismatch
	// should be returned as a wrapped error.
	//
	// The implementer can choose between basing the answer strictly on e.g.
	// v1.Namespace objects that exist in the system, or just the set of
	// different namespaces that have been set on any object belonging to
	// the given GroupKind.
	ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error)

	// ListObjectIDs returns a list of unversioned ObjectIDs.
	// For namespaced GroupKinds, the caller must provide a namespace, and for
	// root-spaced GroupKinds, the caller must not. When namespaced, this function
	// must only return object IDs for that given namespace. If any of the given
	// rules are violated, ErrNamespacedMismatch should be returned as a wrapped error.
	ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) ([]core.UnversionedObjectID, error)
}

// ObjectInfo is the return value from Storage.Stat(). It provides the
// user with information about the given Object, e.g. its ContentType,
// a checksum, and its relative path on disk, if the Storage is a
// FilesystemStorage.
type ObjectInfo interface {
	// ContentTyped returns the ContentType of the Object when stored.
	serializer.ContentTyped
	// ChecksumContainer knows how to retrieve the checksum of the file.
	ChecksumContainer
	// Path is the relative path between the AferoContext root dir and
	// the Stat'd file.
	Path() string
	// ID returns the ID for the given Object.
	ID() core.UnversionedObjectID
}

// ChecksumContainer is an interface for exposing a checksum.
//
// What the checksum is is application-dependent, however, it
// should be the same for two invocations, as long as the stored
// data is the same. It might change over time although the
// underlying data did not. Examples of checksums that can be
// used is: the file modification timestamp, a sha256sum of the
// file content, or the latest Git commit when the file was
// changed.
//
// Look for documentation on the Storage you are using for more
// details on what checksum algorithm is used.
type ChecksumContainer interface {
	// Checksum returns the checksum of the file.
	Checksum() string
}

// Reader provides the write operations for the Storage.
type Writer interface {
	StorageCommon

	// Write writes the given content to the resource indicated by the ID.
	// Error returns are implementation-specific.
	Write(ctx context.Context, id core.UnversionedObjectID, content []byte) error
	// Delete deletes the resource indicated by the ID.
	// If the resource does not exist, it returns ErrNotFound.
	Delete(ctx context.Context, id core.UnversionedObjectID) error
}

// FilesystemStorage extends Storage by specializing it to operate in a
// filesystem context, and in other words use a FileFinder to locate the
// files to operate on.
type FilesystemStorage interface {
	Storage

	// FileFinder returns the underlying FileFinder used.
	// TODO: Maybe one Storage can have multiple FileFinders?
	FileFinder() FileFinder
}

// FileFinder is a generic implementation for locating files on disk, to be
// used by a FilesystemStorage.
//
// Important: The caller MUST guarantee that the implementation can figure
// out if the GroupKind is namespaced or not by the following check:
//
// namespaced := id.ObjectKey().Namespace != ""
//
// In other words, the caller must enforce a namespace being set for namespaced
// kinds, and namespace not being set for non-namespaced kinds.
type FileFinder interface {
	// Filesystem gets the underlying filesystem abstraction, if
	// applicable.
	Filesystem() core.AferoContext

	// ObjectPath gets the file path relative to the root directory.
	// In order to support a create operation, this function must also return a valid path for
	// files that do not yet exist on disk.
	ObjectPath(ctx context.Context, id core.UnversionedObjectID) (string, error)
	// ObjectAt retrieves the ID based on the given relative file path to fs.
	ObjectAt(ctx context.Context, path string) (core.UnversionedObjectID, error)
	// The FileFinder should be able to resolve the content type for various IDs
	ContentTypeResolver
	// The FileFinder should be able to list namespaces and Object IDs
	Lister
}

// MappedFileFinder is an extension to FileFinder that allows it to have an internal
// cache with mappings between UnversionedObjectID and a ChecksumPath. This allows
// higher-order interfaces to manage Objects in files in an unorganized directory
// (e.g. a Git repo).
//
// Multiple Objects in the same file, or multiple Objects with the
// same ID in multiple files are not supported.
type MappedFileFinder interface {
	FileFinder

	// GetMapping retrieves a mapping in the system.
	GetMapping(ctx context.Context, id core.UnversionedObjectID) (ChecksumPath, bool)
	// SetMapping binds an ID to a physical file path. This operation overwrites
	// any previous mapping for id.
	SetMapping(ctx context.Context, id core.UnversionedObjectID, checksumPath ChecksumPath)
	// ResetMappings replaces all mappings at once to the ones in m.
	ResetMappings(ctx context.Context, m map[core.UnversionedObjectID]ChecksumPath)
	// DeleteMapping removes the mapping for the given id.
	DeleteMapping(ctx context.Context, id core.UnversionedObjectID)
}

// UnstructuredStorage is a raw Storage interface that builds on top
// of FilesystemStorage. It uses an ObjectRecognizer to recognize
// otherwise unknown objects in unstructured files.
// The FilesystemStorage must use a MappedFileFinder underneath.
//
// Multiple Objects in the same file, or multiple Objects with the
// same ID in multiple files are not supported.
type UnstructuredStorage interface {
	FilesystemStorage

	// Sync synchronizes the current state of the filesystem with the
	// cached mappings in the MappedFileFinder.
	Sync(ctx context.Context) error

	// ObjectRecognizer returns the underlying ObjectRecognizer used.
	ObjectRecognizer() core.ObjectRecognizer
	// PathExcluder specifies what paths to not sync
	// TODO: enable this
	// PathExcluder() core.PathExcluder
	// MappedFileFinder returns the underlying MappedFileFinder used.
	MappedFileFinder() MappedFileFinder
}

// ChecksumPath is a tuple of a given Checksum and relative file Path,
// for use in MappedFileFinder.
type ChecksumPath struct {
	// TODO: Implement ChecksumContainer, or make ChecksumPath a
	// sub-interface of ObjectID?
	Checksum string
	// Note: path is relative to the AferoContext.
	Path string
}