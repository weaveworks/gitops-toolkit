package frame

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// ErrUnsupportedContentType is returned if the specified content type isn't supported
	ErrUnsupportedContentType = errors.New("unsupported content type")
)

// MakeUnsupportedContentTypeError returns a wrapped ErrUnsupportedContentType along with
// context in a normalized way.
func MakeUnsupportedContentTypeError(ct ContentType) error {
	return fmt.Errorf("%w: %q", ErrUnsupportedContentType, ct)
}

// ContentType specifies the content type for Writers and Readers
type ContentType string

const (
	// ContentTypeJSON specifies usage of JSON as the content type.
	// It is an alias for k8s.io/apimachinery/pkg/runtime.ContentTypeJSON
	ContentTypeJSON = ContentType(runtime.ContentTypeJSON)

	// ContentTypeYAML specifies usage of YAML as the content type.
	// It is an alias for k8s.io/apimachinery/pkg/runtime.ContentTypeYAML
	ContentTypeYAML = ContentType(runtime.ContentTypeYAML)
)

func (ct ContentType) ContentType() ContentType     { return ct }
func (ct ContentType) ToContentTyped() ContentTyped { return &contentTyped{ct} }

// ContentTyped is an interface for objects that are specific to a ContentType.
type ContentTyped interface {
	// ContentType returns the supported/used/relevant ContentType (e.g. ContentTypeYAML or ContentTypeJSON).
	ContentType() ContentType
}

// ContentType implements the ContentTyped interface.
var _ ContentTyped = ContentType("")

// contentTyped is a helper struct that implements the ContentTyped interface.
type contentTyped struct{ contentType ContentType }

func (ct *contentTyped) ContentType() ContentType { return ct.contentType }
