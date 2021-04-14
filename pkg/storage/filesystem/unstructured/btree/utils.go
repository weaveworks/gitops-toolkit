package btree

import "fmt"

// GetValueString searches the Index for an element that is equal to the
// search parameter. The function tries to cast the ValueItem's Value
// to either a string or fmt.Stringer, whose value is then returned. If
// this is unsuccessful, or the item doesn't exist, an empty string is returned.
// If the search is successful, this function returns true.
func GetValueString(index Index, search AbstractItem) (string, bool) {
	it, ok := index.Get(search)
	if !ok {
		return "", false
	}
	valItem := it.GetValueItem()
	if valItem == nil {
		return "", true
	}
	switch s := valItem.Value().(type) {
	case string:
		return s, true
	case fmt.Stringer:
		return s.String(), true
	}
	return "", true
}

// UniqueIterFunc is used in ListUnique.
type UniqueIterFunc func(it ValueItem) string

// ListUnique traverses the index in ascending order for each item under prefix.
// However, when an item is matched, the UniqueIterFunc return value decides where to
// start the search the next time. One possible implementation is to return the
// name of common part you don't want to see again (e.g. "aa:" in the example below),
// which will make ListUnique skip all other "duplicate" "foo:aa:*" items.
//
// Example:
// index = {"bar:aa", "foo:aa:bb", "foo:aa:cc", "foo:aa:cc:dd", "foo:bb:cc", "foo:bb:dd", "foo:dd:ee"}
// prefix = "foo:"
// iterator returns exclusive == true, and strings.Split(it.Key)[1], e.g. "foo:aa:cc:dd" => "aa"
// Then the following items will be visited: {"foo:aa:bb", "foo:bb:cc", "foo:dd:ee"}
func ListUnique(index Index, prefix string, iterator UniqueIterFunc) {
	it, found := index.Find(PrefixQuery(prefix))
	if !found {
		return
	}
	q := NewPrefixPivotQuery(prefix, iterator(it.GetValueItem())).(*prefixPivotQuery)

	for {
		it, found := index.Find(q)
		if !found {
			break
		}
		q.Pivot = iterator(it.GetValueItem())
	}
}
