package btree

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestListUnique(t *testing.T) {
	allItems := []string{"bar:aa", "foo:aa:bb", "foo:aa:cc", "foo:aa:cc:dd", "foo:aaaa:bla", "foo:bb:cc", "foo:bb:dd", "foo:dd:ee", "xyz:foo"}
	tests := []struct {
		items         []string
		prefix        string
		withEndingSep bool
		want          []string
	}{
		// Note the difference between these examples:
		{
			items:         allItems,
			prefix:        "foo:",
			withEndingSep: true,
			want:          []string{"foo:aa:bb", "foo:aaaa:bla", "foo:bb:cc", "foo:dd:ee"},
		},
		{
			items:         allItems,
			prefix:        "foo:",
			withEndingSep: false,
			want:          []string{"foo:aa:bb", "foo:bb:cc", "foo:dd:ee"},
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			i := newIndex(nil)
			for _, item := range tt.items {
				i.Put(NewStringStringItem("", item, ""))
			}

			endingSep := ""
			if tt.withEndingSep {
				endingSep = ":"
			}

			got := make([]string, 0, len(tt.want))
			ListUnique(i, tt.prefix, func(it ValueItem) string {
				str := it.GetValueItem().String()
				got = append(got, str)
				return strings.Split(strings.TrimPrefix(str, tt.prefix), ":")[0] + endingSep
			})

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TestListUnique() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetValueString(t *testing.T) {
	tests := []struct {
		key    string
		value  string
		search string
		want   string
		found  bool
	}{
		{
			key:    "foo:bar",
			value:  "hello",
			search: "foo:bar",
			want:   "hello",
			found:  true,
		},
		{
			key:    "foo:bar",
			value:  "hello",
			search: "notfound",
			want:   "",
			found:  false,
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			i := newIndex(nil)
			i.Put(NewStringStringItem("", tt.key, tt.value))
			got, found := GetValueString(i, PrefixQuery(tt.search))
			if got != tt.want {
				t.Errorf("GetValueString() = %v, want %v", got, tt.want)
			}
			if found != tt.found {
				t.Errorf("GetValueString() = %v, want %v", found, tt.found)
			}
		})
	}
}
