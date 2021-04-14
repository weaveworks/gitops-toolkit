package storage

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
	Namespacer() Namespacer
	// Exists checks if the resource indicated by the ID exists.
	Exists(ctx context.Context, id core.UnversionedObjectID) bool
}

// Namespacer is an interface that lets the caller know if a GroupKind is namespaced
// or not. There are two ready-made implementations:
// 1. kube.RESTMapperToNamespacer
// 2. NewStaticNamespacer
type Namespacer interface {
	// IsNamespaced returns true if the GroupKind is a namespaced type
	IsNamespaced(gk core.GroupKind) (bool, error)
}

// Reader provides the read operations for the Storage.
type Reader interface {
	StorageCommon

	// Read returns a resource's content based on the ID.
	// If the resource does not exist, it returns core.NewErrNotFound.
	Read(ctx context.Context, id core.UnversionedObjectID) ([]byte, error)

	// Checksum returns a checksum of the Object with the given ID.
	//
	// What the checksum is is application-dependent, however, it
	// should be the same for two invocations, as long as the stored
	// data is the same. It might change over time although the
	// underlying data did not. Examples of checksums that can be
	// used is: the file modification timestamp, a sha256sum of the
	// file content, or the latest Git commit when the file was
	// changed.
	Checksum(ctx context.Context, id core.UnversionedObjectID) (string, error)

	// ContentType returns the content type that should be used when serializing
	// the object with the given ID. This operation must function also before the
	// Object with the given id exists in the system, in order to be able to
	// create new Objects.
	ContentType(ctx context.Context, id core.UnversionedObjectID) (serializer.ContentType, error)

	// List operations
	Lister
}

type Lister interface {
	// ListGroupKinds returns all known GroupKinds by the implementation at that
	// time. The set might vary over time as data is created and deleted; and
	// should not be treated as an universal "what types could possibly exist",
	// but more generally, "what are the GroupKinds of the objects that currently
	// exist"? However, obviously, specific implementations might honor this
	// guideline differently. This might be used for introspection into the system.
	ListGroupKinds(ctx context.Context) ([]core.GroupKind, error)

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
	ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) (core.UnversionedObjectIDSet, error)
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
