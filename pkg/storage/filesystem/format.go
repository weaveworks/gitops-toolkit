package filesystem

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/weaveworks/libgitops/pkg/serializer"
)

var (
	ErrCannotDetermineContentType = errors.New("cannot determine content type")
	ErrUnrecognizedContentType    = errors.New("unrecognized content type")
)

// ContentTyper resolves the Content Type of a file given its path and the afero
// filesystem abstraction, so that it is possible to even examine the file if needed
// for making the judgement. See DefaultContentTyper for a sample implementation.
type ContentTyper interface {
	// ContentTypeForPath should return the content type for the file that exists in
	// the given Filesystem (path is relative). If the content type cannot be determined
	// please return a wrapped ErrCannotDetermineContentType error.
	ContentTypeForPath(ctx context.Context, fs Filesystem, path string) (serializer.ContentType, error)
}

// DefaultContentTypes describes the default connection between
// file extensions and a content types.
var DefaultContentTyper ContentTyper = ContentTypeForExtension{
	".json": serializer.ContentTypeJSON,
	".yaml": serializer.ContentTypeYAML,
	".yml":  serializer.ContentTypeYAML,
}

// ContentTypeForExtension implements the ContentTyper interface
// by looking up the extension of the given path in ContentTypeForPath
// matched against the key of the map. The extension in the map key
// must start with a dot, e.g. ".json". The value of the map contains
// the corresponding content type. There might be many extensions which
// map to the same content type, e.g. both ".yaml" -> ContentTypeYAML
// and ".yml" -> ContentTypeYAML.
type ContentTypeForExtension map[string]serializer.ContentType

func (m ContentTypeForExtension) ContentTypeForPath(ctx context.Context, _ Filesystem, path string) (serializer.ContentType, error) {
	ct, ok := m[filepath.Ext(path)]
	if !ok {
		return serializer.ContentType(""), fmt.Errorf("%w for file %q", ErrCannotDetermineContentType, path)
	}
	return ct, nil
}

// StaticContentTyper always responds with the same, statically-set, ContentType for any path.
type StaticContentTyper struct {
	// ContentType is a required field
	ContentType serializer.ContentType
}

func (t StaticContentTyper) ContentTypeForPath(_ context.Context, _ Filesystem, _ string) (serializer.ContentType, error) {
	if len(t.ContentType) == 0 {
		return "", fmt.Errorf("StaticContentTyper.ContentType must not be empty")
	}
	return t.ContentType, nil
}

// FileExtensionResolver knows how to resolve what file extension to use for
// a given ContentType.
type FileExtensionResolver interface {
	// ContentTypeExtension returns the file extension for the given ContentType.
	// The returned string MUST start with a dot, e.g. ".json". If the given
	// ContentType is not known, it is recommended to return a wrapped
	// ErrUnrecognizedContentType.
	ExtensionForContentType(ct serializer.ContentType) (string, error)
}

// DefaultFileExtensionResolver describes a default connection between
// the file extensions and ContentTypes , namely JSON -> ".json" and
// YAML -> ".yaml".
var DefaultFileExtensionResolver FileExtensionResolver = ExtensionForContentType{
	serializer.ContentTypeJSON: ".json",
	serializer.ContentTypeYAML: ".yaml",
}

// ExtensionForContentType is a simple map implementation of FileExtensionResolver.
type ExtensionForContentType map[serializer.ContentType]string

func (m ExtensionForContentType) ExtensionForContentType(ct serializer.ContentType) (string, error) {
	ext, ok := m[ct]
	if !ok {
		return "", fmt.Errorf("%q: %q", ErrUnrecognizedContentType, ct)
	}
	return ext, nil
}
