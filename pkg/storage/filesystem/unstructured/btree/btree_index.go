package btree

import (
	"strings"

	"github.com/google/btree"
)

// AbstractItem is the abstract btree.Item, the ultimate base type for the B-Tree's ordering.
type AbstractItem = btree.Item

// ItemString extends the abstract btree.Item with the "opinion" that all Items in this
// B-Tree have a string representation that is operated on as the B-Tree key.
// It should obey the following logic in ItemString.Less(than):
// - If than is an other ItemString, just perform a "return me < than"
// - If than is an ItemQuery, let the ItemQuery decide the ordering by calling than.QueryGTE(me)
type ItemString interface {
	AbstractItem
	// String returns the string representation of the given item, this serves as the B-Tree key
	String() string
}

// ItemQuery represents a query for the Index, where the user doesn't know the exact string
// representation of the item that is being searched for. The ItemQuery.Less function should
// function just as a "return me < than". However, when comparing an ItemString and an ItemQuery,
// the ItemQuery can fully decide the ordering, because the ItemString delegates the decision to
// the ItemQuery's QueryGTE function. This allows for flexible searching for items in the tree.
//
// When an ordering has been settled, e.g. ItemString1 < ItemQuery <= ItemString2 < ItemString3, and
// Index.Find() is called, ItemString2 will be returned (i.e. the "next item to the right").
// When Index.List() is called, the iterator will be called for all it (ascending "to the
// right") for which ItemQuery.Matches(it) is true.
//
// ItemQueries are never persisted in the tree, they are only used for traversing the tree.
type ItemQuery interface {
	AbstractItem

	// ItemQuery.QueryGTE(ItemString) is the same as (actually called from) ItemString.Less(ItemQuery).
	QueryGTE(it ItemString) bool
	// Matches returns true if the query matches the given item. It is used when iterating, after an
	// ordering has been finalized.
	Matches(it ItemString) bool
}

// Item is the base type that is stored in the B-Tree. There are two main types of Items, ValueItems
// and indexed pointers to ValueItems. Hence, any Item points to a ValueItem in one way or an other.
type Item interface {
	ItemString
	// GetValueItem returns the ValueItem this Item points to (in the case of an index), or itself
	// (in the case of Item already being a ValueItem).
	GetValueItem() ValueItem
}

// ItemIterator represents a callback function when iterating through a set of items in the tree.
// As long as true is returned, iteration continues.
type ItemIterator func(it Item) bool

// ValueItem represents a mapped key to a value that is stored in the B-Tree.
type ValueItem interface {
	Item

	// Key returns the key of this mapping
	Key() interface{}
	// Value returns the value of this mapping
	Value() interface{}
	// IndexedPtrs returns all indexed items that are pointing to this ValueItem
	IndexedPtrs() []Item
}

// Index represents one B-Tree that contains key-value mappings indexed by their key and possibly
// other fields.
type Index interface {
	// Get returns an Item in the tree that matches exactly it (i.e. !it.Less(it2) && !it2.Less(it))
	// Both an ItemString (or higher) or an ItemQuery can be passed to this function.
	Get(it AbstractItem) (Item, bool)
	// Put inserts or overwrites the given ValueItem (including related indexes) in the underlying tree.
	Put(it ValueItem)
	// Delete deletes the ValueItem (and the related indexes) that is equal to it. True is returned if
	// such an item actually existed in the tree and was deleted.
	Delete(it AbstractItem) bool

	// Find returns the next item in ascending order, when the place for the ItemQuery q has been found as:
	// Item1 < q <= Item2 < Item3
	// In this example, (Item2, true) would be returned, as long as q.Matches(Item2) == true. Otherwise, or
	// if q is the maximum of the tree, (nil, false) is returned.
	// See PrefixQuery and prefixPivotQuery for examples.
	Find(q ItemQuery) (Item, bool)
	// List returns the next items in ascending order, when the place for the ItemQuery q has been found as:
	// Item1 < q <= Item2 < Item3 < Item4
	// In this example, it in [Item2, Item4] would be iterated, as long as q.Matches(it) == true. When false
	// is returned from a match, iteration is stopped.
	// See PrefixQuery and prefixPivotQuery for examples.
	List(q ItemQuery, iterator ItemIterator)

	// Clear clears the B-Tree completely, but re-uses some nodes for better resource utilization.
	// It does not disturb other trees that share the same Copy-on-Write base.
	Clear()

	// Internal returns the underlying B-Tree.
	Internal() *btree.BTree
}

type bTreeIndexImpl struct {
	btree     *btree.BTree
	parentRef string
}

// Get returns an Item in the tree that matches exactly it (i.e. !it.Less(it2) && !it2.Less(it))
// Both an ItemString (or higher) or an ItemQuery can be passed to this function.
func (i *bTreeIndexImpl) Get(it btree.Item) (Item, bool) {
	found := i.btree.Get(it)
	if found != nil {
		return found.(Item), true
	}
	return nil, false
}

// Put inserts or overwrites the given ValueItem (including related indexes) in the underlying tree.
func (i *bTreeIndexImpl) Put(it ValueItem) {
	// First, delete any previous, now stale, data related to this item
	i.deleteIndexes(it)
	// Add the item to the tree
	i.btree.ReplaceOrInsert(it)
	// Register all indexes of it, too
	for _, idxPtr := range it.IndexedPtrs() {
		i.btree.ReplaceOrInsert(idxPtr)
	}
}

// Delete deletes the ValueItem (and the related indexes) that is equal to it. True is returned if
// such an item actually existed in the tree and was deleted.
func (i *bTreeIndexImpl) Delete(it btree.Item) bool {
	// deleteIndexes returns true if it exists (=> needs to be deleted)
	if !i.deleteIndexes(it) {
		return false // nothing to delete
	}

	// Delete the item itself from the tree
	i.btree.Delete(it)
	return true
}

// deleteIndexes deletes the indexes associated with it
// true is returned if the deletions were made, false
// if the item did not exist
func (i *bTreeIndexImpl) deleteIndexes(it btree.Item) bool {
	// Deliberately Get the item first, to resolve the ValueItem it points to
	found, ok := i.Get(it)
	if !ok {
		return false // nothing to delete, not found
	}

	// Delete all indexes of it
	for _, idxPtr := range found.GetValueItem().IndexedPtrs() {
		i.btree.Delete(idxPtr)
	}
	return true
}

// Find returns the next item in ascending order, when the place for the ItemQuery q has been found as:
// Item1 < q <= Item2 < Item3
// In this example, (Item2, true) would be returned, as long as q.Matches(Item2) == true. Otherwise, or
// if q is the maximum of the tree, (nil, false) is returned.
func (i *bTreeIndexImpl) Find(q ItemQuery) (retit Item, found bool) {
	i.list(q, func(it Item) bool {
		retit = it
		found = true
		return false // only find one item
	})
	return // retit, found
}

// List returns the next items in ascending order, when the place for the ItemQuery q has been found as:
// Item1 < q <= Item2 < Item3 < Item4
// In this example, it in [Item2, Item4] would be iterated, as long as q.Matches(it) == true. When false
// is returned from a match, iteration is stopped.
func (i *bTreeIndexImpl) List(q ItemQuery, iterator ItemIterator) {
	i.list(q, iterator)
}

func (i *bTreeIndexImpl) list(q ItemQuery, iterator ItemIterator) {
	var ii Item // cache ii between iteration callbacks
	i.btree.AscendGreaterOrEqual(q, func(i btree.Item) bool {
		ii = i.(Item)
		if !q.Matches(ii) { // make sure ii matches the query
			return false
		}
		return iterator(ii)
	})
}

func (i *bTreeIndexImpl) Internal() *btree.BTree { return i.btree }
func (i *bTreeIndexImpl) Clear()                 { i.btree.Clear(true) }

// NewItemString returns a new ItemString for the given B-Tree key.
// Custom ValueItems should embed this ItemString to automatically get
// the expected sorting functionality.
func NewItemString(key string) ItemString {
	return &itemString{key}
}

// itemString implements ItemString
var _ ItemString = &itemString{}

type itemString struct{ key string }

// Less implements the sorting functionality described in the ItemString godoc.
// If this Item is compared to an ItemQuery, the ItemQuery should decide the ordering.
// If this Item is compared to a fellow ItemString, just use simple string comparison.
func (s *itemString) Less(item btree.Item) bool {
	switch it := item.(type) {
	case ItemQuery:
		return it.QueryGTE(s)
	case ItemString:
		return s.key < it.String()
	default:
		panic("items must implement either ItemQuery or ItemString")
	}
}
func (s *itemString) String() string { return s.key }

// NewIndexedPtr returns a new Item that for the given key, points to
// the given ValueItem. This means fields of ValueItems can be indexed
// using the following key, and added to the B-Tree. ptr must be non-nil
// otherwise this function will panic. The key of the pointed-to item
// will be appended to the sort key as well
func NewIndexedPtr(key string, ptr *ValueItem) Item {
	if ptr == nil {
		panic("NewIndexedPtr: ptr must not be nil")
	}
	return &indexedPtr{NewItemString(key + ":" + (*ptr).String()), ptr}
}

// indexedPtr implements Item.
var _ Item = &indexedPtr{}

// indexedPtr extends the ItemString with the given pointer to the ValueItem.
type indexedPtr struct {
	ItemString
	ptr *ValueItem
}

func (s *indexedPtr) GetValueItem() ValueItem { return *s.ptr }

// PrefixQuery implements ItemQuery
var _ ItemQuery = PrefixQuery("")

// PrefixQuery is an ItemQuery that matches all items with the given prefix. For a Find() the smallest
// item with the given prefix is returned. For a List() the items containing the prefix will be iterated
// in ascending order (from smallest to largest).
// Example: bar:xx < foo:aa:aa < foo:aa:bb < foo:bb:aa < xx:yy:zz
// Find("foo:aa") => "foo:aa:aa"
// List("foo:aa") => {"foo:aa:aa", "foo:aa:bb"}
// Find("foo:bb") => "foo:bb:aa"
// List("foo:bb") => {"foo:bb:aa"}
type PrefixQuery string

func (s PrefixQuery) Less(item btree.Item) bool   { return string(s) < item.(ItemString).String() }
func (s PrefixQuery) QueryGTE(it ItemString) bool { return it.String() < string(s) }

func (s PrefixQuery) Matches(it ItemString) bool {
	return strings.HasPrefix(it.String(), string(s))
}

// NewPrefixPivotQuery returns an ItemQuery that matches all items with the given Prefix, but starting
// the search for items that don't start with "Prefix+Pivot". A Find() returns the smallest item that does
// not have the "Prefix+Pivot" prefix, but still contains "Prefix". A List() starts iterating the tree
// in ascending order (from smallest to largest) from the item returned by Find(). Behavior is undefined
// if Prefix or Pivot (or both) is an empty string.
//
// Example: bar:xx < foo:aa:aa < foo:aa:bb < foo:bb:aa < foo:bb:cc < foo:cc:zz < xx:yy:zz
// Find(Prefix: "foo:", Pivot: "aa") => "foo:bb:aa"
// List(Prefix: "foo:", Pivot: "aa") => {"foo:bb:aa", "foo:bb:cc", "foo:cc:zz"}
// Find(Prefix: "foo:", Pivot: "bb") => "foo:cc:zz"
// List(Prefix: "foo:", Pivot: "bb") => {"foo:cc:zz"}
func NewPrefixPivotQuery(prefix, pivot string) ItemQuery {
	return &prefixPivotQuery{Prefix: prefix, Pivot: pivot}
}

// prefixPivotQuery implements ItemQuery
var _ ItemQuery = &prefixPivotQuery{}

type prefixPivotQuery struct {
	Prefix string
	Pivot  string
}

func (s *prefixPivotQuery) key() string { return s.Prefix + s.Pivot }
func (s *prefixPivotQuery) Less(item btree.Item) bool {
	itStr := item.(ItemString).String()
	return s.key() < itStr && !strings.HasPrefix(itStr, s.key())
}

func (s *prefixPivotQuery) QueryGTE(it ItemString) bool {
	b := it.String() < s.key() || strings.HasPrefix(it.String(), s.key())
	return b
}
func (s *prefixPivotQuery) Matches(it ItemString) bool {
	return strings.HasPrefix(it.String(), s.Prefix)
}

// NewStringStringItem returns a new mapping (between a string-encoded key and value), that
// can be stored in the B-Tree. Keys stored under the same "bucket" prefix together essentially
// form a "virtual" map[string]string within the B-Tree, but with e.g. copy-on-write support.
// Any extra indexed fields registered will point to this ValueItem.
func NewStringStringItem(bucket, key, value string, indexedFields ...string) ValueItem {
	str := key
	if len(bucket) != 0 {
		str = bucket + ":" + key
	}
	kvItem := &kvValueItem{
		ItemString: NewItemString(str),
		key:        key,
		value:      value,
		indexes:    make([]Item, 0, len(indexedFields)),
	}
	var valit ValueItem = kvItem
	for _, indexedField := range indexedFields {
		kvItem.indexes = append(kvItem.indexes, NewIndexedPtr(indexedField, &valit))
	}
	return kvItem
}

type kvValueItem struct {
	ItemString
	key, value string
	indexes    []Item
}

func (i *kvValueItem) GetValueItem() ValueItem { return i }         // this is already a ValueItem
func (i *kvValueItem) Key() interface{}        { return i.key }     // just return the plain key
func (i *kvValueItem) Value() interface{}      { return i.value }   // just return the plain value
func (i *kvValueItem) IndexedPtrs() []Item     { return i.indexes } // indexes from constructor
