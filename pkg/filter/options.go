package filter

import "sigs.k8s.io/controller-runtime/pkg/client"

// FilterOption is an interface for implementations that know how to
// mutate FilterOptions.
type FilterOption interface {
	// ApplyToFilterOptions applies the configuration of the current object into a target FilterOptions struct.
	ApplyToFilterOptions(target *FilterOptions)
}

// FilterOptions is a set of options for filtering. It implements the ObjectFilter interface
// itself, so it can be used kind of as a multi-ObjectFilter.
type FilterOptions struct {
	// ObjectFilters contains a set of filters for a single object. All of the filters must return
	// true an a nil error for Match(obj) to return (true, nil).
	ObjectFilters []ObjectFilter
}

// Match matches the object against all the ObjectFilters.
func (o *FilterOptions) Match(obj client.Object) (bool, error) {
	for _, filter := range o.ObjectFilters {
		matched, err := filter.Match(obj)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

// ApplyToFilterOptions implements FilterOption
func (o *FilterOptions) ApplyToFilterOptions(target *FilterOptions) {
	target.ObjectFilters = append(target.ObjectFilters, o.ObjectFilters...)
}

// ApplyOptions applies the given FilterOptions to itself and returns itself.
func (o *FilterOptions) ApplyOptions(opts []FilterOption) *FilterOptions {
	for _, opt := range opts {
		opt.ApplyToFilterOptions(o)
	}
	return o
}

// ApplyOption applies one option that aims to implement FilterOption,
// but at compile-time maybe does not for sure. This can be used for
// lists of other Options that possibly implement FilterOption in the
// following way: for _, opt := range opts { filterOpts.ApplyOption(opt) }
func (o *FilterOptions) ApplyOption(opt interface{}) *FilterOptions {
	if fOpt, ok := opt.(FilterOption); ok {
		fOpt.ApplyToFilterOptions(o)
	}
	return o
}
