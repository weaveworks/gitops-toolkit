package filter

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NamespaceFilter implements ObjectFilter and FilterOption.
// It also implements client.{List,DeleteAllOf}Option so
// it can be passed into client.Client.{List,DeleteAllOf}
// as a way to conveniently filter those lists.
var _ ObjectFilter = NamespaceFilter{}
var _ FilterOption = NamespaceFilter{}
var _ client.ListOption = NamespaceFilter{}
var _ client.DeleteAllOfOption = NamespaceFilter{}

// NamespaceFilter is an ObjectFilter that compares Object.GetNamespace()
// to the Namespace field.
type NamespaceFilter struct {
	// Namespace matches the object by .metadata.namespace. If left as
	// an empty string, it is ignored when filtering.
	// +required
	Namespace string
}

// Match implements ObjectFilter
func (f NamespaceFilter) Match(obj client.Object) (bool, error) {
	// Require f.Namespace to always be set.
	if len(f.Namespace) == 0 {
		return false, fmt.Errorf("the NamespaceFilter.Namespace field must not be empty: %w", ErrInvalidFilterParams)
	}
	// Otherwise, just use an equality check
	return f.Namespace == obj.GetNamespace(), nil
}

// ApplyToList implements client.ListOption, but is just a "dummy" implementation in order to implement
// the interface, so that this struct can be passed to client.Reader.List()
func (f NamespaceFilter) ApplyToList(_ *client.ListOptions)               {}
func (f NamespaceFilter) ApplyToDeleteAllOf(_ *client.DeleteAllOfOptions) {}

// ApplyToFilterOptions implements FilterOption
func (f NamespaceFilter) ApplyToFilterOptions(target *FilterOptions) {
	target.ObjectFilters = append(target.ObjectFilters, f)
}
