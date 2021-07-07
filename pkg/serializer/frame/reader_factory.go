package frame

import (
	"context"
	"io"
)

// DefaultFactory is the default variant of Factory capable
// of creating YAML- and JSON-compatible Readers and Writers.
//
// If ReaderWriterOptions.MaxFrames == 1, any ContentType can
// be supplied, not just YAML or JSON. ReadFrame will then read
// and return all data in the underlying io.Reader. WriteFrame
// will write the given frame to the underlying io.Writer as-is.
type DefaultFactory struct{}

func (f DefaultFactory) NewReader(contentType ContentType, r io.Reader, opts ...ReaderOption) Reader {
	// Build the options from the defaults
	o := defaultReaderOpts().ApplyOptions(opts)
	// Wrap r in a io.NopCloser if it isn't closable. Mark os.Std{in,out,err} as not closable.
	rc, hasCloser := toReadCloser(r)
	// Wrap the low-level Reader from lowlevelFromReadCloser in a composite highlevelReader applying common policy
	return newHighlevelReader(f.lowlevelFromReadCloser(contentType, rc, o), hasCloser, o)
}

func toReadCloser(r io.Reader) (rc io.ReadCloser, hasCloser bool) {
	rc, hasCloser = r.(io.ReadCloser)
	// Don't mark os.Std{in,out,err} as closable
	if isStdio(rc) {
		hasCloser = false
	}
	if !hasCloser {
		rc = io.NopCloser(r)
	}
	return rc, hasCloser
}

func (DefaultFactory) lowlevelFromReadCloser(contentType ContentType, rc io.ReadCloser, o *ReaderOptions) Reader {
	switch contentType {
	case ContentTypeYAML:
		return newYAMLReader(rc, o)
	case ContentTypeJSON:
		return newJSONReader(rc, o)
	default:
		// If only one frame is allowed, there is no need to frame.
		if o.MaxFrames == 1 {
			return newSingleReader(contentType, rc, o)
		}
		return newErrReader(contentType, MakeUnsupportedContentTypeError(contentType))
	}
}

// defaultReaderFactory is the variable used in public methods.
var defaultReaderFactory ReaderFactory = DefaultFactory{}

// NewReader returns a Reader for the given ContentType and underlying io.Read(Clos)er.
//
// This is a shorthand for DefaultFactory{}.NewReader(contentType, r, opts...)
func NewReader(contentType ContentType, r io.Reader, opts ...ReaderOption) Reader {
	return defaultReaderFactory.NewReader(contentType, r, opts...)
}

// NewYAMLReader returns a Reader that supports both YAML and JSON. Frames are separated by "---\n"
//
// This is a shorthand for NewReader(ContentTypeYAML, rc, opts...)
func NewYAMLReader(r io.Reader, opts ...ReaderOption) Reader {
	return NewReader(ContentTypeYAML, r, opts...)
}

// NewJSONReader returns a Reader that supports both JSON. Objects are read from the stream one-by-one,
// each object making up its own frame.
//
// This is a shorthand for NewReader(ContentTypeJSON, rc, opts...)
func NewJSONReader(r io.Reader, opts ...ReaderOption) Reader {
	return NewReader(ContentTypeJSON, r, opts...)
}

func newErrReader(contentType ContentType, err error) Reader {
	return &errReader{contentType.ToContentTyped(), &nopCloser{}, err}
}

// errReader always returns an error
type errReader struct {
	ContentTyped
	Closer
	err error
}

func (fr *errReader) ReadFrame(context.Context) ([]byte, error) { return nil, fr.err }

// nopCloser returns nil when Close(ctx) is called
type nopCloser struct{}

func (*nopCloser) Close(context.Context) error { return nil }
