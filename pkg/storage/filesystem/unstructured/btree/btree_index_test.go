package btree

import (
	"reflect"
	"strconv"
	"testing"
)

func Test_ItemString_Less_ItemString_QueryPrefix(t *testing.T) {
	tests := []struct {
		str  string
		than string
		want bool
	}{
		{"", "", false},
		{"", "foo", true},
		{"foo", "", false},
		{"a", "b", true},
		{"a:a", "a:b", true},
		{"a:c", "a:b", false},
		{"b:a", "a:b", false},
		{"id:Bar.foo.com", "path:sample-file.yaml", true},
		{"id:Bar.foo.com", "checksum:123", false},
		{"path:sample-file.yaml:key:Baz.foo.com:default:foo:sample-file.yaml", "path:sample-file.yaml:key:Bar.foo.com:custom:foo:sample-file.yaml", false},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if got := NewItemString(tt.str).Less(NewItemString(tt.than)); got != tt.want {
				t.Errorf("NewItemString.Less(NewItemString) = %v, want %v", got, tt.want)
			}
			if got := NewItemString(tt.str).Less(PrefixQuery(tt.than)); got != tt.want {
				t.Errorf("NewItemString.Less(PrefixQuery) = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ItemString_Less_QueryPrefixPivot(t *testing.T) {
	tests := []struct {
		str           string
		prefix, pivot string
		want          bool
	}{
		{"", "foo", "", true},
		{"a", "b", "", true},
		{"a:a", "a:b", "", true},
		{"a:c", "a:b", "", false},
		{"b:a", "a:b", "", false},
		{"id:Bar.foo.com", "path:sample-file.yaml", "", true},
		{"id:Bar.foo.com", "checksum:123", "", false},
		{"path:sample-file.yaml:key:Baz.foo.com:default:foo:sample-file.yaml", "path:sample-file.yaml:key:Bar.foo.com:custom:foo:sample-file.yaml", "", false},
		//  bar:xx < foo:aa:aa < foo:aa:bb < foo:bb:aa < foo:bb:cc < foo:cc:zz < xx:yy:zz
		{"bar:xx", "foo:aa", "aa", true},
		{"foo:", "foo:aa", "aa", true},
		{"foo:aa:aa", "foo:", "aa", true},
		{"foo:aa:bb", "foo:", "aa", true},
		{"foo:bb:aa", "foo:", "aa", false},
		{"foo:cc:aa", "foo:", "aa", false},
		{"foo:bb:bb", "foo:", "bb", true},
		{"foo:cc:aa", "foo:", "bb", false},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if got := NewItemString(tt.str).Less(NewPrefixPivotQuery(tt.prefix, tt.pivot)); got != tt.want {
				t.Errorf("ItemString.Less() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_bTreeIndexImpl_Find(t *testing.T) {
	exampleItems := []string{"bar:xx", "foo:aa:aa", "foo:aa:bb", "foo:bb:aa", "foo:bb:cc", "foo:cc:zz", "xx:yy:zz"}
	tests := []struct {
		items     []string
		q         ItemQuery
		wantItem  string
		wantFound bool
	}{
		// Test cases for PrefixQuery:
		{
			items:     exampleItems,
			q:         PrefixQuery(""),
			wantItem:  "bar:xx",
			wantFound: true,
		},
		{
			// Find("foo:aa") => "foo:aa:aa"
			items:     exampleItems,
			q:         PrefixQuery("foo:aa"),
			wantItem:  "foo:aa:aa",
			wantFound: true,
		},
		{
			// Find("foo:bb") => "foo:bb:aa"
			items:     exampleItems,
			q:         PrefixQuery("foo:bb"),
			wantItem:  "foo:bb:aa",
			wantFound: true,
		},
		// Test cases for PrefixPivotQuery:
		{
			// Find(Prefix: "foo:", Pivot: "aa") => "foo:bb:aa"
			items:     exampleItems,
			q:         NewPrefixPivotQuery("foo:", "aa"),
			wantItem:  "foo:bb:aa",
			wantFound: true,
		},
		{
			// Find(Prefix: "foo:", Pivot: "bb") => "foo:cc:zz"
			items:     exampleItems,
			q:         NewPrefixPivotQuery("foo:", "bb"),
			wantItem:  "foo:cc:zz",
			wantFound: true,
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			i := newIndex(nil)
			for _, item := range tt.items {
				i.Put(NewStringStringItem("", item, ""))
			}
			gotItem, gotFound := i.Find(tt.q)
			if gotItem.String() != tt.wantItem {
				t.Errorf("bTreeIndexImpl.Find() gotRetit = %v, want %v", gotItem.String(), tt.wantItem)
			}
			if gotFound != tt.wantFound {
				t.Errorf("bTreeIndexImpl.Find() gotFound = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

func Test_Queries_List(t *testing.T) {
	exampleItems := []string{"bar:xx", "foo:aa:aa", "foo:aa:bb", "foo:bb:aa", "foo:bb:cc", "foo:cc:zz", "xx:yy:zz"}
	tests := []struct {
		items []string
		q     ItemQuery
		want  []string
	}{
		// Test cases for PrefixQuery:
		{
			items: exampleItems,
			q:     PrefixQuery(""),
			want:  exampleItems,
		},
		{
			// List("foo:aa") => {"foo:aa:aa", "foo:aa:bb"}
			items: exampleItems,
			q:     PrefixQuery("foo:aa"),
			want:  exampleItems[1:3],
		},
		{
			// List("foo:bb") => {"foo:bb:aa", "foo:bb:cc"}
			items: exampleItems,
			q:     PrefixQuery("foo:bb"),
			want:  exampleItems[3:5],
		},
		// Test cases for PrefixPivotQuery:
		{
			// List(Prefix: "foo:", Pivot: "aa") => {"foo:bb:aa", "foo:bb:cc", "foo:cc:zz"}
			items: exampleItems,
			q:     NewPrefixPivotQuery("foo:", "aa"),
			want:  exampleItems[3:6],
		},
		{
			// List(Prefix: "foo:", Pivot: "bb") => {"foo:cc:zz"}
			items: exampleItems,
			q:     NewPrefixPivotQuery("foo:", "bb"),
			want:  exampleItems[5:6],
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			i := newIndex(nil)
			for _, item := range tt.items {
				i.Put(NewStringStringItem("", item, ""))
			}
			got := make([]string, 0, len(tt.want))
			i.List(tt.q, func(it Item) bool {
				got = append(got, it.String())
				return true
			})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bTreeIndexImpl.List() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_index_List(t *testing.T) {
	var (
		key1 = NewStringStringItem("id", "Bar.foo.com:default:foo", "sample-file.yaml", "path:sample-file.yaml")
		key2 = NewStringStringItem("id", "Bar.foo.com:default:other-foo", "other-file.yaml", "path:other-file.yaml")
		key3 = NewStringStringItem("id", "Bar.foo.com:custom:foo", "sample-file.yaml", "path:sample-file.yaml")
		key4 = NewStringStringItem("id", "Baz.foo.com:default:foo", "sample-file.yaml", "path:sample-file.yaml")
	)
	sampleInit := func(i Index) {
		i.Put(key1)
		i.Put(key2)
		i.Put(key3)
		i.Put(key4)
	}
	sampleCleanup := func(i Index) {
		i.Delete(key1)
		i.Delete(key2)
		i.Delete(key3)
		i.Delete(key4)
	}
	tests := []struct {
		initFunc    func(i Index)
		cleanupFunc func(i Index)
		prefix      string
		want        []ValueItem
	}{
		{
			initFunc:    sampleInit,
			cleanupFunc: sampleCleanup,
			prefix:      "path",
			want:        []ValueItem{key2, key3, key1, key4}, // sorted in order of the index, i.e. the files, and THEN the actual values
		},
		{
			initFunc:    sampleInit,
			cleanupFunc: sampleCleanup,
			prefix:      "path:sample-file.yaml",
			want:        []ValueItem{key3, key1, key4},
		},
		{
			initFunc:    sampleInit,
			cleanupFunc: sampleCleanup,
			prefix:      "id:Bar.foo.com",
			want:        []ValueItem{key3, key1, key2},
		},
		{
			initFunc:    sampleInit,
			cleanupFunc: sampleCleanup,
			prefix:      "id:Baz.foo.com",
			want:        []ValueItem{key4},
		},
		{
			initFunc:    sampleInit,
			cleanupFunc: sampleCleanup,
			prefix:      "id:Bar.foo.com:default",
			want:        []ValueItem{key1, key2},
		},
		{
			initFunc:    sampleInit,
			cleanupFunc: sampleCleanup,
			prefix:      "id:Bar.foo.com:default:foo",
			want:        []ValueItem{key1},
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			btreeIndex := newIndex(nil)
			tt.initFunc(btreeIndex)
			wantStr := make([]string, 0, len(tt.want))
			for _, it := range tt.want {
				wantStr = append(wantStr, it.String())
			}

			got := []string{}
			btreeIndex.List(PrefixQuery(tt.prefix), func(it Item) bool {
				got = append(got, it.GetValueItem().String())
				return true
			})
			if !reflect.DeepEqual(got, wantStr) {
				t.Errorf("got = %v, want %v", got, wantStr)
			}
			tt.cleanupFunc(btreeIndex)
			if l := btreeIndex.Internal().Len(); l != 0 {
				if !reflect.DeepEqual(got, wantStr) {
					t.Errorf("expected clean tree, got len = %d", l)
				}
			}
		})
	}
}
