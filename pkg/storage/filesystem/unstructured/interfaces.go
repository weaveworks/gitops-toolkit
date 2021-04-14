package unstructured

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
)

// Storage is a raw Storage interface that builds on top
// of Storage. It uses an ObjectRecognizer to recognize
// otherwise unknown objects in unstructured files.
// The Storage must use a unstructured.FileFinder underneath.
type Storage interface {
	filesystem.Storage

	// Sync synchronizes the current state of the filesystem, and overwrites all
	// previously cached mappings in the unstructured.FileFinder. "successful"
	// mappings returned are those that are observed to be distinct. "duplicates"
	// contains such IDs that weren't distinct; but existed in multiple files.
	Sync(ctx context.Context) (successful, duplicates core.UnversionedObjectIDSet, err error)

	// ObjectRecognizer returns the underlying ObjectRecognizer used.
	ObjectRecognizer() ObjectRecognizer
	// FrameReaderFactory returns the underlying FrameReaderFactory used.
	FrameReaderFactory() serializer.FrameReaderFactory
	// PathExcluder specifies what paths to not sync. Can possibly be nil.
	PathExcluder() filesystem.PathExcluder
	// UnstructuredFileFinder returns the underlying unstructured.FileFinder used.
	UnstructuredFileFinder() FileFinder
}

// ObjectRecognizer recognizes objects stored in files.
type ObjectRecognizer interface {
	// RecognizeObjectIDs returns the ObjectIDs present in the file with the given name,
	// content type and content (in the FrameReader).
	RecognizeObjectIDs(fileName string, fr serializer.FrameReader) ([]core.ObjectID, error)
}

// FileFinder is an extension to filesystem.FileFinder that allows it to have an internal
// cache with mappings between an UnversionedObjectID and a ChecksumPath. This allows
// higher-order interfaces to manage Objects in files in an unorganized directory
// (e.g. a Git repo).
//
// This implementation supports multiple IDs per file, and can deal with duplicate IDs across
// distinct file paths. This implementation supports looking at the context for VersionRef info.
type FileFinder interface {
	filesystem.FileFinder

	// SetMapping sets all the IDs that are stored in this path, for the given, updated checksum.
	// ids must be the exact set of ObjectIDs that are observed at the given path; the previously-stored
	// list will be overwritten. The new checksum will be recorded in the system for this path.
	// The "added" set will record what IDs didn't exist before and were added. "duplicates" are IDs that
	// were technically added, but already existed, mapped to other files in the system. Other files'
	// mappings aren't removed in this function, but no new duplicates are added to this path.
	// Instead such duplicates are returned instead. "removed" contains the set of IDs that existed
	// previously, but were now removed.
	// If ids is an empty set; all mappings to the given path will be removed, and "removed" will contain
	// all prior mappings. (In fact, this is what DeleteMapping does.)
	//
	// ID sets are computed as follows (none of the sets overlap with each other):
	//
	// {ids} => {added} + {duplicates} + {removed} + {modified}
	//
	// {oldIDs} - {removed} + {added} => {newIDs}
	SetMapping(ctx context.Context, state ChecksumPath, ids core.UnversionedObjectIDSet) (added, duplicates, removed core.UnversionedObjectIDSet)

	// ResetMappings removes all prior data and sets all given mappings at once.
	// Duplicates are NOT stored in the cache at all for this operation, instead they are returned.
	ResetMappings(ctx context.Context, mappings map[ChecksumPath]core.UnversionedObjectIDSet) (duplicates core.UnversionedObjectIDSet)

	// DeleteMapping removes a mapping for a given path to a file. Previously-stored IDs are returned.
	DeleteMapping(ctx context.Context, path string) (removed core.UnversionedObjectIDSet)

	// ChecksumForPath retrieves the latest known checksum for the given path.
	ChecksumForPath(ctx context.Context, path string) (string, bool)

	// MoveFile moves an internal mapping from oldPath to newPath. moved == true if the oldPath
	// existed and hence the move was performed.
	MoveFile(ctx context.Context, oldPath, newPath string) (moved bool)

	// RegisterVersionRef registers a new "head" version ref, based (using copy-on-write logic),
	// on the existing versionref "base". head must be non-nil, but base can be nil, if it is
	// desired that "head" has no parent, and hence, is blank. An error is returned if head is
	// nil, or base does not exist.
	RegisterVersionRef(head, base core.VersionRef) error
	// HasVersionRef returns true if the given head version ref has been registered.
	HasVersionRef(head core.VersionRef) bool
	// DeleteVersionRef deletes the given head version ref.
	DeleteVersionRef(head core.VersionRef)
}

// ChecksumPath is a tuple of a given Checksum and relative file Path,
// for use in unstructured.FileFinder.
type ChecksumPath struct {
	// Checksum is the checksum of the file at the given path.
	//
	// What the checksum is is application-dependent, however, it
	// should be the same for two invocations, as long as the stored
	// data is the same. It might change over time although the
	// underlying data did not. Examples of checksums that can be
	// used is: the file modification timestamp, a sha256sum of the
	// file content, or the latest Git commit when the file was
	// changed.
	//
	// The checksum is calculated by the filesystem.Filesystem.
	Checksum string
	// Path to the file, relative to filesystem.Filesystem.RootDirectory().
	Path string
}
