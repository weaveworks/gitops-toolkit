package filter

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UIDFilter implements ObjectFilter and FilterOption.
// It also implements client.{List,DeleteAllOf}Option so
// it can be passed into client.Client.{List,DeleteAllOf}
// as a way to conveniently filter those lists.
var _ ObjectFilter = UIDFilter{}
var _ FilterOption = UIDFilter{}
var _ client.ListOption = UIDFilter{}
var _ client.DeleteAllOfOption = UIDFilter{}

// UIDFilter is an ObjectFilter that compares Object.GetUID() to
// the UID field by either equality or prefix. The UID field is required,
// otherwise ErrInvalidFilterParams is returned.
type UIDFilter struct {
	// UID matches the object by .metadata.uid.
	// +required
	UID types.UID
	// MatchPrefix whether the UID-matching should be exact, or prefix-based.
	// +optional
	MatchPrefix bool
}

// Match implements ObjectFilter
func (f UIDFilter) Match(obj client.Object) (bool, error) {
	// Require f.UID to always be set.
	if len(f.UID) == 0 {
		return false, fmt.Errorf("the UIDFilter.UID field must not be empty: %w", ErrInvalidFilterParams)
	}
	// If the UID should be matched by the prefix, use strings.HasPrefix
	if f.MatchPrefix {
		return strings.HasPrefix(string(obj.GetUID()), string(f.UID)), nil
	}
	// Otherwise, just use an equality check
	return f.UID == obj.GetUID(), nil
}

// ApplyToList implements client.ListOption, but is just a "dummy" implementation in order to implement
// the interface, so that this struct can be passed to client.Reader.List()
func (f UIDFilter) ApplyToList(_ *client.ListOptions)               {}
func (f UIDFilter) ApplyToDeleteAllOf(_ *client.DeleteAllOfOptions) {}

// ApplyToFilterOptions implements FilterOption
func (f UIDFilter) ApplyToFilterOptions(target *FilterOptions) {
	target.ObjectFilters = append(target.ObjectFilters, f)
}
