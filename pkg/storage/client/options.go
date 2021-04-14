package client

import (
	"github.com/weaveworks/libgitops/pkg/filter"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExtendedListOption interface {
	client.ListOption
	filter.FilterOption
}

type ExtendedListOptions struct {
	client.ListOptions
	filter.FilterOptions
}

var _ ExtendedListOption = &ExtendedListOptions{}

func (o *ExtendedListOptions) ApplyToList(target *client.ListOptions) {
	o.ListOptions.ApplyToList(target)
}

func (o *ExtendedListOptions) ApplyToFilterOptions(target *filter.FilterOptions) {
	o.FilterOptions.ApplyToFilterOptions(target)
}

func (o *ExtendedListOptions) ApplyOptions(opts []client.ListOption) *ExtendedListOptions {
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

type ExtendedDeleteAllOfOption interface {
	ExtendedListOption
	client.DeleteAllOfOption
}

type ExtendedDeleteAllOfOptions struct {
	ExtendedListOptions
	client.DeleteOptions
}

var _ ExtendedDeleteAllOfOption = &ExtendedDeleteAllOfOptions{}

func (o *ExtendedDeleteAllOfOptions) ApplyToDeleteAllOf(target *client.DeleteAllOfOptions) {
	o.DeleteOptions.ApplyToDelete(&target.DeleteOptions)
}

func (o *ExtendedDeleteAllOfOptions) ApplyOptions(opts []client.DeleteAllOfOption) *ExtendedDeleteAllOfOptions {
	// Cannot directly apply to o, hence, create a temporary object to which upstream opts are applied
	do := (&client.DeleteAllOfOptions{}).ApplyOptions(opts)
	o.ExtendedListOptions.ListOptions = do.ListOptions
	o.DeleteOptions = do.DeleteOptions

	// Apply all FilterOptions, if they implement that interface
	for _, opt := range opts {
		o.FilterOptions.ApplyOption(opt)
	}
	return o
}
