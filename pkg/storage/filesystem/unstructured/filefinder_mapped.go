package unstructured

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/unstructured/btree"
	utilerrs "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// ErrNotTracked is returned when the requested resource wasn't found.
	ErrNotTracked = errors.New("untracked object")
	// ErrTrackingDuplicate is returned when a duplicate of two object IDs in the cache have occurred
	ErrTrackingDuplicate = errors.New("duplicate object ID; already exists in an other file")
)

// GenericFileFinder implements FileFinder.
var _ FileFinder = &GenericFileFinder{}

// NewGenericFileFinder creates a new instance of GenericFileFinder,
// that implements the FileFinder interface. The contentTyper is optional,
// by default core.DefaultContentTyper will be used.
func NewGenericFileFinder(contentTyper filesystem.ContentTyper, fs filesystem.Filesystem) FileFinder {
	if contentTyper == nil {
		contentTyper = filesystem.DefaultContentTyper
	}
	if fs == nil {
		panic("NewGenericFileFinder: fs is mandatory")
	}
	return &GenericFileFinder{
		contentTyper: contentTyper,
		fs:           fs,
		index:        btree.NewVersionedIndex(),
		mu:           &sync.RWMutex{},
	}
}

// GenericFileFinder is a generic implementation of FileFinder.
// It uses a ContentTyper to identify what content type a file uses.
//
// This implementation relies on that all information about what files exist
// is fed through {Set,Reset}Mapping. If a file or ID is requested that doesn't
// exist in the internal cache, ErrNotTracked will be returned.
//
// Hence, this implementation does not at the moment support creating net-new
// Objects without someone calling SetMapping() first.
type GenericFileFinder struct {
	// Default: DefaultContentTyper
	contentTyper filesystem.ContentTyper
	fs           filesystem.Filesystem

	index btree.VersionedIndex
	// mu guards index
	mu *sync.RWMutex
}

func (f *GenericFileFinder) Filesystem() filesystem.Filesystem {
	return f.fs
}

func (f *GenericFileFinder) ContentTyper() filesystem.ContentTyper {
	return f.contentTyper
}

func (f *GenericFileFinder) versionedIndex(ctx context.Context) (btree.Index, error) {
	i, ok := f.index.VersionedTree(core.GetVersionRef(ctx).Branch())
	if ok {
		return i, nil
	}
	return nil, fmt.Errorf("no such versionref registered")
}

// ObjectPath gets the file path relative to the root directory
func (f *GenericFileFinder) ObjectPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		return "", err
	}

	// Lookup the BTree item for the given ID
	p, ok := index.Get(queryObject(id))
	if !ok {
		return "", utilerrs.NewAggregate([]error{ErrNotTracked, core.NewErrNotFound(id)})
	}
	// Return the path
	return p.GetValueItem().Value().(string), nil
}

// ObjectsAt retrieves the ObjectIDs in the file with the given relative file path.
func (f *GenericFileFinder) ObjectsAt(ctx context.Context, path string) (core.UnversionedObjectIDSet, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		return nil, err
	}
	idSet := f.objectsAt(index, path)
	// Error if there is no such known path
	if idSet.Len() == 0 {
		// TODO: Support "creation" of Objects easier, in a generic way through an interface, e.g.
		// NewObjectPlacer?
		return nil, fmt.Errorf("%q: %w", path, ErrNotTracked)
	}
	return idSet, nil
}

func (f *GenericFileFinder) objectsAt(index btree.Index, path string) core.UnversionedObjectIDSet {
	// Traverse the objects belonging to the given path index
	ids := core.NewUnversionedObjectIDSet()
	index.List(queryPath(path), func(it btree.Item) bool {
		// Insert each objectID belonging to that path into the set
		ids.Insert(it.GetValueItem().Key().(core.UnversionedObjectID))
		return true
	})
	return ids
}

// ListGroupKinds returns all known GroupKinds by the implementation at that
// time. The set might vary over time as data is created and deleted; and
// should not be treated as an universal "what types could possibly exist",
// but more generally, "what are the GroupKinds of the objects that currently
// exist"? However, obviously, specific implementations might honor this
// guideline differently. This might be used for introspection into the system.
func (f *GenericFileFinder) ListGroupKinds(ctx context.Context) ([]core.GroupKind, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		return nil, err
	}

	gks := []core.GroupKind{}
	// List GroupKinds directly under "id:*"
	prefix := idField + ":"
	// Extract the GroupKind from the visited item, and return the groupkind, so it
	// won't be visited again
	btree.ListUnique(index, prefix, func(it btree.ValueItem) string {
		gk := it.Key().(core.UnversionedObjectID).GroupKind()
		gks = append(gks, gk)
		return gk.String() + ":" // note: important to return this, see btree/utils_test.go why
	})
	return gks, nil
}

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
func (f *GenericFileFinder) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		return nil, err
	}

	nsSet := sets.NewString()
	// List namespaces under "id:{groupkind}:*"
	prefix := idForGroupKind(gk)
	// Extract the namespace from the visited item, and return the groupkind exclusively, so it
	// won't be visited again
	btree.ListUnique(index, prefix, func(it btree.ValueItem) string {
		ns := it.Key().(core.UnversionedObjectID).ObjectKey().Namespace
		nsSet.Insert(ns)
		return ns + ":" // note: important to return this, see btree/utils_test.go why
	})
	return nsSet, nil
}

// ListObjectIDs returns a list of unversioned ObjectIDs.
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object IDs for that given namespace. If any of the given
// rules are violated, ErrNamespacedMismatch should be returned as a wrapped error.
func (f *GenericFileFinder) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) (core.UnversionedObjectIDSet, error) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		return nil, err
	}

	ids := core.NewUnversionedObjectIDSet()
	// List ObjectIDs under "id:{groupkind}:{ns}:*"
	index.List(queryNamespace(gk, namespace), func(it btree.Item) bool {
		ids.Insert(it.GetValueItem().Key().(core.UnversionedObjectID))
		return true
	})
	return ids, nil
}

// ChecksumForPath retrieves the latest known checksum for the given path.
func (f *GenericFileFinder) ChecksumForPath(ctx context.Context, path string) (string, bool) {
	// Lock for reading
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		return "", false
	}
	return btree.GetValueString(index, queryChecksum(path))
}

// MoveFile moves an internal mapping from oldPath to newPath. moved == true if the oldPath
// existed and hence the move was performed.
func (f *GenericFileFinder) MoveFile(ctx context.Context, oldPath, newPath string) bool {
	// Lock for writing
	f.mu.Lock()
	defer f.mu.Unlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		logrus.Debugf("MoveFile %s -> %s: got error from versionedIndex: %v", oldPath, newPath, err)
		return false
	}

	// Get all the ObjectIDs assigned to the old path
	idSet := f.objectsAt(index, oldPath)
	logrus.Tracef("MoveFile: idSet: %s", idSet)

	// Re-assign the IDs to the new path
	_ = idSet.ForEach(func(id core.UnversionedObjectID) error {
		index.Put(newIDItem(id, newPath))
		return nil
	})

	// Move the checksum info over by
	// a) getting the checksum for the old path
	// b) assigning that checksum to the new path
	// c) deleting the item for the old path
	checksum, ok := btree.GetValueString(index, queryChecksum(oldPath))
	if !ok {
		logrus.Error("MoveFile: Expected checksum to be available, but wasn't")
		// if this happens; newPath won't be mapped to any checksum; but nothing worse
	}
	index.Put(newChecksumItem(newPath, checksum))
	index.Delete(newChecksumItem(newPath, checksum))
	logrus.Tracef("MoveFile: Moved checksum from %q to %q", oldPath, newPath)

	return true
}

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
func (f *GenericFileFinder) SetMapping(ctx context.Context, state ChecksumPath, newIDs core.UnversionedObjectIDSet) (added, duplicates, removed core.UnversionedObjectIDSet) {
	// Lock for writing
	f.mu.Lock()
	defer f.mu.Unlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		// Always return an empty set, although the version ref does not exist
		added = core.NewUnversionedObjectIDSet()
		duplicates = core.NewUnversionedObjectIDSet()
		removed = core.NewUnversionedObjectIDSet()
		return
	}

	return f.setIDsAtPath(index, state.Path, state.Checksum, newIDs)
}

// internal method; not using any mutex; caller's responsibility
func (f *GenericFileFinder) setIDsAtPath(index btree.Index, path, checksum string, newIDs core.UnversionedObjectIDSet) (added, duplicates, removed core.UnversionedObjectIDSet) {
	// If there are no new ids, delete the checksum mapping
	if newIDs.Len() == 0 {
		index.Delete(queryChecksum(path))
	} else {
		// Update the checksum.
		index.Put(newChecksumItem(path, checksum))
	}

	// Get the old IDs; and compute the different "buckets"
	oldIDs := f.objectsAt(index, path)
	logrus.Tracef("setIDsAtPath: oldIDs: %s", oldIDs)
	// Get newID entries that are not present in oldIDs
	added = newIDs.Difference(oldIDs)
	logrus.Tracef("setIDsAtPath: added: %s", added)

	duplicates = core.NewUnversionedObjectIDSet()

	// Get oldIDs entries that are not present in newIDs
	removed = oldIDs.Difference(newIDs)
	logrus.Tracef("setIDsAtPath: removed: %s", removed)

	// Register the added items
	_ = added.ForEach(func(addedID core.UnversionedObjectID) error {
		itemToAdd := newIDItem(addedID, path)
		// Check if this ID already exists in some other file. TODO: Is the second check needed?
		if otherFile, _ := btree.GetValueString(index, itemToAdd); len(otherFile) != 0 && otherFile != path {
			// If so; it is a duplicate; move it to duplicates
			added.Delete(addedID)
			duplicates.Insert(addedID)
			return nil
		}
		// If it didn't exist somewhere else, add the mapping between this ID and path
		index.Put(itemToAdd)
		return nil
	})

	logrus.Tracef("setIDsAtPath: added post-filter: %s", added)
	logrus.Tracef("setIDsAtPath: duplicates post-filter: %s", duplicates)

	// Remove the removed items
	_ = removed.ForEach(func(removedID core.UnversionedObjectID) error {
		index.Delete(queryObject(removedID))
		return nil
	})

	// return the different buckets
	return added, duplicates, removed
}

// DeleteMapping removes a mapping for a given path to a file. Previously-stored IDs are returned.
func (f *GenericFileFinder) DeleteMapping(ctx context.Context, path string) (removed core.UnversionedObjectIDSet) {
	// Lock for writing
	f.mu.Lock()
	defer f.mu.Unlock()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		// Always return an empty set, although the version ref does not exist
		removed = core.NewUnversionedObjectIDSet()
		return
	}

	// Re-use the setMappings internal function
	_, _, removed = f.setIDsAtPath(
		index,                            // Get the versioned index
		path,                             // Delete mappings at this path
		"",                               // No checksum -> delete that mapping
		core.NewUnversionedObjectIDSet(), // Empty "desired state" -> everything removed
	)
	return
}

// ResetMappings removes all prior data and sets all given mappings at once.
// Duplicates are NOT stored in the cache at all for this operation, instead they are returned.
func (f *GenericFileFinder) ResetMappings(ctx context.Context, m map[ChecksumPath]core.UnversionedObjectIDSet) (duplicates core.UnversionedObjectIDSet) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Keep track of all duplicates there are in the mappings.
	// Always return an empty set, although the version ref does not exist
	duplicates = core.NewUnversionedObjectIDSet()

	// Get the versioned tree for the context
	index, err := f.versionedIndex(ctx)
	if err != nil {
		return
	}
	// Completely clean up all existing data on the branch before starting.
	index.Clear()
	logrus.Trace("ResetMappings: cleaned branch")

	// Go through all files and add them to the cache
	for cp, allIDs := range m {
		// The first "duplicate" entry will succeed in "making it" to the cache; but all the others
		// will be registered here. After this iteration of set; remove the duplicates completely
		// from the cache.
		logrus.Tracef("ResetMappings: cp %v, allIDs: %s", cp, allIDs)

		// Re-use the internal setMappings function again.
		// We don't need added & removed here, as we know that {allIDs} = {added} + {newDuplicates}
		// Removals is always empty as we cleaned all mappings before we started this method.
		_, newDuplicates, _ := f.setIDsAtPath(index, cp.Path, cp.Checksum, allIDs)
		logrus.Tracef("ResetMappings: newDuplicates: %s", newDuplicates)
		// Add all duplicates together so we can process them later
		duplicates.InsertSet(newDuplicates)
	}

	logrus.Tracef("ResetMappings: total duplicates: %s", duplicates)

	// Go and "fix up" (i.e. delete) the duplicates that were wrongly added previously
	// In the resulting mappings; no duplicates are allowed (to avoid "races" at random
	// between different duplicates otherwise)
	_ = duplicates.ForEach(func(id core.UnversionedObjectID) error {
		index.Delete(queryObject(id))
		return nil
	})

	return
}

// RegisterVersionRef registers a new "head" version ref, based (using copy-on-write logic),
// on the existing versionref "base". head must be non-nil, but base can be nil, if it is
// desired that "head" has no parent, and hence, is blank. An error is returned if head is
// nil, or base does not exist.
func (f *GenericFileFinder) RegisterVersionRef(head, base core.VersionRef) error {
	if head == nil {
		return fmt.Errorf("head must not be nil")
	}
	baseBranch := ""
	if base != nil {
		baseBranch = base.Branch()
	}
	_, err := f.index.NewVersionedTree(head.Branch(), baseBranch)
	return err
}

// HasVersionRef returns true if the given head version ref has been registered.
func (f *GenericFileFinder) HasVersionRef(head core.VersionRef) bool {
	_, ok := f.index.VersionedTree(head.Branch())
	return ok
}

// DeleteVersionRef deletes the given head version ref.
func (f *GenericFileFinder) DeleteVersionRef(head core.VersionRef) {
	f.index.DeleteVersionedTree(head.Branch())
}

func idForGroupKind(gk core.GroupKind) string            { return idField + ":" + gk.String() + ":" }
func idForNamespace(gk core.GroupKind, ns string) string { return idForGroupKind(gk) + ns + ":" }
func queryNamespace(gk core.GroupKind, ns string) btree.ItemQuery {
	return btree.PrefixQuery(idForNamespace(gk, ns))
}

func idForObject(id core.UnversionedObjectID) string {
	return idForNamespace(id.GroupKind(), id.ObjectKey().Namespace) + id.ObjectKey().Name
}
func queryObject(id core.UnversionedObjectID) btree.ItemQuery {
	return btree.PrefixQuery(idForObject(id))
}

func queryPath(path string) btree.ItemQuery     { return btree.PrefixQuery(pathIdxField + ":" + path) }
func queryChecksum(path string) btree.ItemQuery { return btree.PrefixQuery(checksumField + ":" + path) }

func newChecksumItem(path, checksum string) btree.ValueItem {
	return btree.NewStringStringItem(checksumField, path, checksum)
}

func newIDItem(id core.UnversionedObjectID, path string) btree.ValueItem {
	return &idItemImpl{
		ItemString: btree.NewItemString(idForObject(id)),
		id:         id,
		path:       path,
	}
}

type idItemImpl struct {
	btree.ItemString
	id   core.UnversionedObjectID
	path string
}

const (
	idField       = "id"
	pathIdxField  = "path"
	checksumField = "chk"
)

func (i *idItemImpl) GetValueItem() btree.ValueItem { return i }
func (i *idItemImpl) Key() interface{}              { return i.id }
func (i *idItemImpl) Value() interface{}            { return i.path }

func (i *idItemImpl) IndexedPtrs() []btree.Item {
	var self btree.ValueItem = i
	return []btree.Item{
		btree.NewIndexedPtr(pathIdxField+":"+i.path, &self),
	}
}
