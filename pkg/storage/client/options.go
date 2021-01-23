package client

import (
	"github.com/weaveworks/libgitops/pkg/filter"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ListOption interface {
	client.ListOption
	filter.FilterOption
}

type ListOptions struct {
	client.ListOptions
	filter.FilterOptions
}

var _ ListOption = &ListOptions{}

func (o *ListOptions) ApplyToList(target *client.ListOptions) {
	o.ListOptions.ApplyToList(target)
}

func (o *ListOptions) ApplyToFilterOptions(target *filter.FilterOptions) {
	o.FilterOptions.ApplyToFilterOptions(target)
}

func (o *ListOptions) ApplyOptions(opts []client.ListOption) *ListOptions {
	// Apply the "normal" ListOptions
	o.ListOptions.ApplyOptions(opts)
	// Apply all FilterOptions, if they implement that interface
	for _, opt := range opts {
		o.FilterOptions.ApplyOption(opt)
	}

	// If listOpts.Namespace was given, add it to the list of ObjectFilters
	if len(o.Namespace) != 0 {
		o.ObjectFilters = append(o.ObjectFilters, filter.NamespaceFilter{Namespace: o.Namespace})
	}
	// If listOpts.LabelSelector was given, add it to the list of ObjectFilters
	if o.LabelSelector != nil {
		o.ObjectFilters = append(o.ObjectFilters, filter.LabelsFilter{LabelSelector: o.LabelSelector})
	}

	return o
}

type DeleteAllOfOption interface {
	ListOption
	client.DeleteAllOfOption
}

type DeleteAllOfOptions struct {
	ListOptions
	client.DeleteOptions
}

var _ DeleteAllOfOption = &DeleteAllOfOptions{}

func (o *DeleteAllOfOptions) ApplyToDeleteAllOf(target *client.DeleteAllOfOptions) {
	o.DeleteOptions.ApplyToDelete(&target.DeleteOptions)
}

func (o *DeleteAllOfOptions) ApplyOptions(opts []client.DeleteAllOfOption) *DeleteAllOfOptions {
	// Cannot directly apply to o, hence, create a temporary object to which upstream opts are applied
	do := (&client.DeleteAllOfOptions{}).ApplyOptions(opts)
	o.ListOptions.ListOptions = do.ListOptions
	o.DeleteOptions = do.DeleteOptions

	// Apply all FilterOptions, if they implement that interface
	for _, opt := range opts {
		o.FilterOptions.ApplyOption(opt)
	}
	return o
}