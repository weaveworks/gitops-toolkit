package filter

import (
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NameFilter implements ObjectFilter and FilterOption.
// It also implements client.{List,DeleteAllOf}Option so
// it can be passed into client.Client.{List,DeleteAllOf}
// as a way to conveniently filter those lists.
var _ ObjectFilter = NameFilter{}
var _ FilterOption = NameFilter{}
var _ client.ListOption = NameFilter{}
var _ client.DeleteAllOfOption = NameFilter{}

// NameFilter is an ObjectFilter that compares Object.GetName()
// to the Name field by either equality or prefix.
type NameFilter struct {
	// Name matches the object by .metadata.name.
	// +required
	Name string
	// MatchPrefix whether the name matching should be exact, or prefix-based.
	// +optional
	MatchPrefix bool
}

// Match implements ObjectFilter
func (f NameFilter) Match(obj client.Object) (bool, error) {
	// Require f.Name to always be set.
	if len(f.Name) == 0 {
		return false, fmt.Errorf("the NameFilter.Name field must not be empty: %w", ErrInvalidFilterParams)
	}

	// If the Name should be matched by the prefix, use strings.HasPrefix
	if f.MatchPrefix {
		return strings.HasPrefix(obj.GetName(), f.Name), nil
	}
	// Otherwise, just use an equality check
	return f.Name == obj.GetName(), nil
}

// ApplyToList implements client.ListOption, but is just a "dummy" implementation in order to implement
// the interface, so that this struct can be passed to client.Reader.List()
func (f NameFilter) ApplyToList(_ *client.ListOptions)               {}
func (f NameFilter) ApplyToDeleteAllOf(_ *client.DeleteAllOfOptions) {}

// ApplyToFilterOptions implements FilterOption
func (f NameFilter) ApplyToFilterOptions(target *FilterOptions) {
	target.ObjectFilters = append(target.ObjectFilters, f)
}
