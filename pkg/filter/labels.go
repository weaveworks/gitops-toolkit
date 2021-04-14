package filter

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LabelsFilter implements ObjectFilter and FilterOption.
// It also implements client.{List,DeleteAllOf}Option so
// it can be passed into client.Client.{List,DeleteAllOf}
// as a way to conveniently filter those lists.
var _ ObjectFilter = LabelsFilter{}
var _ FilterOption = LabelsFilter{}
var _ client.ListOption = LabelsFilter{}
var _ client.DeleteAllOfOption = LabelsFilter{}

// LabelsFilter is an ObjectFilter that compares metav1.Object.GetLabels()
// to the LabelSelector field.
type LabelsFilter struct {
	// LabelSelector filters results by label.  Use SetLabelSelector to
	// set from raw string form.
	// +required
	LabelSelector labels.Selector
}

// Match implements ObjectFilter
func (f LabelsFilter) Match(obj client.Object) (bool, error) {
	// Require f.Namespace to always be set.
	if f.LabelSelector == nil {
		return false, fmt.Errorf("the LabelsFilter.LabelSelector field must not be nil: %w", ErrInvalidFilterParams)
	}

	return f.LabelSelector.Matches(labels.Set(obj.GetLabels())), nil
}

// ApplyToList implements client.ListOption, but is just a "dummy" implementation in order to implement
// the interface, so that this struct can be passed to client.Reader.List()
func (f LabelsFilter) ApplyToList(_ *client.ListOptions)               {}
func (f LabelsFilter) ApplyToDeleteAllOf(_ *client.DeleteAllOfOptions) {}

// ApplyToFilterOptions implements FilterOption
func (f LabelsFilter) ApplyToFilterOptions(target *FilterOptions) {
	target.ObjectFilters = append(target.ObjectFilters, f)
}
