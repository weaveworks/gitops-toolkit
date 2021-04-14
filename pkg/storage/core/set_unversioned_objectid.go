package core

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
)

// UnversionedObjectIDSet is a set of UnversionedObjectIDs.
// The underlying data storage is a map[UnversionedObjectID]struct{}.
//
// This interface should be as similar as possible to
// k8s.io/apimachinery/pkg/util/sets.
type UnversionedObjectIDSet interface {
	// Has returns true if the object ID is in the set.
	Has(id UnversionedObjectID) bool
	// HasAny returns true if any of the object IDs are in the set.
	HasAny(ids ...UnversionedObjectID) bool

	// Insert inserts the given object IDs into the set. Returns itself.
	// WARNING: This mutates the receiver. Issue a Copy() before if not desired.
	Insert(ids ...UnversionedObjectID) UnversionedObjectIDSet
	// InsertSet inserts the contents of s2 into itself, and returns itself.
	// WARNING: This mutates the receiver. Issue a Copy() before if not desired.
	InsertSet(s2 UnversionedObjectIDSet) UnversionedObjectIDSet

	// Delete deletes the given object IDs from the set. Returns itself.
	// WARNING: This mutates the receiver. Issue a Copy() before if not desired.
	Delete(ids ...UnversionedObjectID) UnversionedObjectIDSet
	// DeleteSet deletes the contents of s2 from itself, and returns itself.
	// WARNING: This mutates the receiver. Issue a Copy() before if not desired.
	DeleteSet(s2 UnversionedObjectIDSet) UnversionedObjectIDSet

	// List lists the given object IDs of the set, in no particular order.
	// List requires O(n) extra memory, when n == Len(). Use ForEach for no copying.
	List() []UnversionedObjectID
	// ForEach runs fn for each item in the set. Does not copy the whole list.
	// Uses a for-range underneath, so it is even safe to delete items underneath, ref:
	// https://stackoverflow.com/questions/23229975/is-it-safe-to-remove-selected-keys-from-map-within-a-range-loop
	// If an error occurs, the rest of the IDs are not traversed. Iteration order is random.
	ForEach(fn func(id UnversionedObjectID) error) error

	// Len returns the length of the set
	Len() int
	// Copy does a shallow copy of set element; but performs a deep copy of the
	// underlying map itself; so mutating operations don't propagate unwantedly.
	Copy() UnversionedObjectIDSet

	// Difference returns a set of objects that are not in s2
	// For example:
	// s1 = {a1, a2, a3}
	// s2 = {a1, a2, a4, a5}
	// s1.Difference(s2) = {a3}
	// s2.Difference(s1) = {a4, a5}
	Difference(s2 UnversionedObjectIDSet) UnversionedObjectIDSet

	// String returns a human-friendly representation
	String() string
}

// NewUnversionedObjectIDSet creates a new UnversionedObjectIDSet
func NewUnversionedObjectIDSet(ids ...UnversionedObjectID) UnversionedObjectIDSet {
	return NewUnversionedObjectIDSetSized(len(ids), ids...)
}

// NewUnversionedObjectIDSet creates a new UnversionedObjectIDSet for a given map length.
func NewUnversionedObjectIDSetSized(len int, ids ...UnversionedObjectID) UnversionedObjectIDSet {
	return (make(unversionedObjectIDSet, len)).Insert(ids...)
}

// UnversionedObjectIDSetFromVersionedSlice transforms a slice of ObjectIDs to
// an unversioned set.
func UnversionedObjectIDSetFromVersionedSlice(versioned []ObjectID) UnversionedObjectIDSet {
	result := NewUnversionedObjectIDSetSized(len(versioned))
	for _, id := range versioned {
		// Important: We should "unwrap" to a plain UnversionedObjectID here, so
		// equality works properly in e.g. map keys.
		result.Insert(id.WithoutVersion())
	}
	return result
}

type unversionedObjectIDSet map[UnversionedObjectID]sets.Empty

func (s unversionedObjectIDSet) Has(id UnversionedObjectID) bool {
	_, found := s[id]
	return found
}

func (s unversionedObjectIDSet) HasAny(ids ...UnversionedObjectID) bool {
	for _, id := range ids {
		if s.Has(id) {
			return true
		}
	}
	return false
}

func (s unversionedObjectIDSet) Insert(ids ...UnversionedObjectID) UnversionedObjectIDSet {
	for _, id := range ids {
		s[id] = sets.Empty{}
	}
	return s
}

// InsertSet inserts the contents of s2 into itself, and returns itself.
func (s unversionedObjectIDSet) InsertSet(s2 UnversionedObjectIDSet) UnversionedObjectIDSet {
	_ = s2.ForEach(func(id UnversionedObjectID) error {
		s[id] = sets.Empty{}
		return nil
	})
	return s
}

func (s unversionedObjectIDSet) Delete(ids ...UnversionedObjectID) UnversionedObjectIDSet {
	for _, id := range ids {
		delete(s, id)
	}
	return s
}

// DeleteSet deletes the contents of s2 from itself, and returns itself.
func (s unversionedObjectIDSet) DeleteSet(s2 UnversionedObjectIDSet) UnversionedObjectIDSet {
	_ = s2.ForEach(func(id UnversionedObjectID) error {
		delete(s, id)
		return nil
	})
	return s
}

func (s unversionedObjectIDSet) List() []UnversionedObjectID {
	list := make([]UnversionedObjectID, 0, len(s))
	for id := range s {
		list = append(list, id)
	}
	return list
}

// ForEach runs fn for each item in the set. Does not copy the whole list.
func (s unversionedObjectIDSet) ForEach(fn func(id UnversionedObjectID) error) (err error) {
	for key := range s {
		if err = fn(key); err != nil {
			return
		}
	}
	return
}

func (s unversionedObjectIDSet) Len() int {
	return len(s)
}

func (s unversionedObjectIDSet) Copy() UnversionedObjectIDSet {
	result := make(unversionedObjectIDSet, s.Len())
	for id := range s {
		result.Insert(id)
	}
	return result
}

// Difference returns a set of objects that are not in s2
// For example:
// s1 = {a1, a2, a3}
// s2 = {a1, a2, a4, a5}
// s1.Difference(s2) = {a3}
// s2.Difference(s1) = {a4, a5}
func (s unversionedObjectIDSet) Difference(s2 UnversionedObjectIDSet) UnversionedObjectIDSet {
	result := NewUnversionedObjectIDSet()
	for key := range s {
		if !s2.Has(key) {
			result.Insert(key)
		}
	}
	return result
}

func (s unversionedObjectIDSet) String() string {
	return fmt.Sprintf("UnversionedObjectIDSet (len=%d): %v", s.Len(), s.List())
}
