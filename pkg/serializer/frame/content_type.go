package frame

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// ErrUnsupportedFramingType is returned if the specified framing type isn't supported
	ErrUnsupportedFramingType = errors.New("unsupported framing type")
)

// MakeUnsupportedFramingTypeError returns a wrapped ErrUnsupportedFramingType along with
// context in a normalized way.
func MakeUnsupportedFramingTypeError(ct FramingType) error {
	return fmt.Errorf("%w: %q", ErrUnsupportedFramingType, ct)
}

// FramingType specifies the framing type for Writers and Readers
type FramingType string

const (
	// FramingTypeJSON specifies usage of JSON as the framing type.
	// It is an alias for k8s.io/apimachinery/pkg/runtime.ContentTypeYAML
	FramingTypeJSON = FramingType(runtime.ContentTypeJSON)

	// FramingTypeYAML specifies usage of YAML as the framing type.
	// It is an alias for k8s.io/apimachinery/pkg/runtime.ContentTypeYAML
	FramingTypeYAML = FramingType(runtime.ContentTypeYAML)
)

func (ct FramingType) FramingType() FramingType     { return ct }
func (ct FramingType) ToFramingTyped() FramingTyped { return &framingTyped{ct} }

// FramingTyped is an interface for objects that are specific to a FramingType.
type FramingTyped interface {
	// FramingType returns the supported/used/relevant FramingType (e.g. FramingTypeYAML or FramingTypeJSON).
	FramingType() FramingType
}

// FramingType implements the FramingTyped interface.
var _ FramingTyped = FramingType("")

// framingTyped is a helper struct that implements the FramingTyped interface.
type framingTyped struct{ framingType FramingType }

func (ct *framingTyped) FramingType() FramingType { return ct.framingType }
