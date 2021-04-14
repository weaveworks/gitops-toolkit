package filesystem

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// Storage (in this filesystem package) extends storage.Storage by specializing it to operate in a
// filesystem context, and in other words use a FileFinder to locate the
// files to operate on.
type Storage interface {
	storage.Storage

	// FileFinder returns the underlying FileFinder used.
	// TODO: Maybe one Storage can have multiple FileFinders?
	FileFinder() FileFinder
}

// FileFinder is a generic implementation for locating files on disk, to be
// used by a Storage.
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
	Filesystem() Filesystem

	// ContentTyper gets the underlying ContentTyper used. The ContentTyper
	// must always return a result although the underlying given path doesn't
	// exist.
	ContentTyper() ContentTyper

	// ObjectPath gets the file path relative to the root directory.
	// In order to support a create operation, this function must also return a valid path for
	// files that do not yet exist on disk.
	ObjectPath(ctx context.Context, id core.UnversionedObjectID) (string, error)
	// ObjectsAt retrieves the ObjectIDs in the file with the given relative file path.
	ObjectsAt(ctx context.Context, path string) (core.UnversionedObjectIDSet, error)
	// The FileFinder should be able to list namespaces and Object IDs
	storage.Lister
}
