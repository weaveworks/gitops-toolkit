package filter

import (
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrInvalidFilterParams describes an error where invalid parameters were given
	// to a filter.
	ErrInvalidFilterParams = errors.New("invalid parameters given to filter")
)

// ObjectFilter is an interface for filtering objects one-by-one.
type ObjectFilter interface {
	// Match takes in one object (at once, per invocation), and returns a
	// boolean whether the object matches the filter parameters, or not.
	Match(obj client.Object) (bool, error)
}
