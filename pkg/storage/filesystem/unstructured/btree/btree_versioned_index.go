package btree

import (
	"errors"
	"fmt"

	"github.com/google/btree"
)

var (
	ErrVersionRefNotFound      = errors.New("version ref tree not found")
	ErrVersionRefAlreadyExists = errors.New("version ref tree already exists")
)

type VersionedIndex interface {
	VersionedTree(ref string) (Index, bool)
	NewVersionedTree(ref, base string) (Index, error)
	DeleteVersionedTree(ref string)
}

func NewVersionedIndex() VersionedIndex {
	return &bTreeVersionedIndexImpl{
		indexes:  make(map[string]Index),
		freelist: btree.NewFreeList(btree.DefaultFreeListSize),
	}
}

type bTreeVersionedIndexImpl struct {
	indexes  map[string]Index
	freelist *btree.FreeList
}

func (i *bTreeVersionedIndexImpl) VersionedTree(ref string) (Index, bool) {
	t, ok := i.indexes[ref]
	return t, ok
}

func (i *bTreeVersionedIndexImpl) NewVersionedTree(ref, base string) (Index, error) {
	// Make sure ref already doesn't exist
	_, ok := i.VersionedTree(ref)
	if ok {
		return nil, fmt.Errorf("%w: %s", ErrVersionRefAlreadyExists, ref)
	}

	var t2 Index
	if len(base) != 0 {
		// Get the base versionref
		t, ok := i.VersionedTree(base)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrVersionRefNotFound, base)
		}
		// Clone the base BTree
		t2 = &bTreeIndexImpl{btree: t.Internal().Clone(), parentRef: base}
	} else {
		// Create a new BTree with the shared freelist
		t2 = newIndex(i.freelist)
	}
	// Register in the map
	i.indexes[ref] = t2
	return t2, nil
}

func (i *bTreeVersionedIndexImpl) DeleteVersionedTree(ref string) {
	t, ok := i.VersionedTree(ref)
	if ok {
		// Move the nodes of the cow-part of the given BTree to the freelist for re-use
		t.Internal().Clear(true)
	}
	// Just delete the index
	delete(i.indexes, ref)
}

func newIndex(freelist *btree.FreeList) Index {
	if freelist == nil {
		return &bTreeIndexImpl{btree: btree.New(32)}
	}
	return &bTreeIndexImpl{btree: btree.NewWithFreeList(32, freelist)}
}
