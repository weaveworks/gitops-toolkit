package frame

import (
	"io"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

// Documentation below attached to NewWriter.
func (f DefaultFactory) NewWriter(framingType FramingType, w io.Writer, opts ...WriterOption) Writer {
	// Build the concrete options struct from defaults and modifiers
	o := defaultWriterOpts().ApplyOptions(opts)
	wc, hasCloser := toWriteCloser(w)
	// Wrap the writer in a layer that provides tracing and mutex capabilities
	return newHighlevelWriter(f.newFromWriteCloser(framingType, wc, o), hasCloser, o)
}

func toWriteCloser(w io.Writer) (wc io.WriteCloser, hasCloser bool) {
	wc, hasCloser = w.(io.WriteCloser)
	if isStdio(wc) {
		hasCloser = false
	}
	if !hasCloser {
		wc = &nopWriteCloser{w}
	}
	return wc, hasCloser
}

func (DefaultFactory) newFromWriteCloser(framingType FramingType, wc io.WriteCloser, o *WriterOptions) Writer {
	switch framingType {
	case FramingTypeYAML:
		return newDelegatingWriter(framingType, json.YAMLFramer.NewFrameWriter(wc), wc, o)
	case FramingTypeJSON:
		return newDelegatingWriter(framingType, json.Framer.NewFrameWriter(wc), wc, o)
	default:
		// If only one frame is allowed, there is no need to frame.
		if o.MaxFrames == 1 {
			return newSingleWriter(framingType, wc, o)
		}
		return newErrWriter(framingType, MakeUnsupportedFramingTypeError(framingType))
	}
}

// defaultWriterFactory is the variable used in public methods.
var defaultWriterFactory WriterFactory = DefaultFactory{}

// NewWriter returns a new Writer for the given Writer and FramingType.
// The returned Writer is thread-safe.
func NewWriter(framingType FramingType, w io.Writer, opts ...WriterOption) Writer {
	return defaultWriterFactory.NewWriter(framingType, w, opts...)
}

// NewYAMLWriter returns a Writer that writes YAML frames separated by "---\n"
//
// This call is the same as NewWriter(FramingTypeYAML, w, opts...)
func NewYAMLWriter(w io.Writer, opts ...WriterOption) Writer {
	return NewWriter(FramingTypeYAML, w, opts...)
}

// NewJSONWriter returns a Writer that writes JSON frames without separation
// (i.e. "{ ... }{ ... }{ ... }" on the wire)
//
// This call is the same as NewWriter(FramingTypeYAML, w)
func NewJSONWriter(w io.Writer, opts ...WriterOption) Writer {
	return NewWriter(FramingTypeJSON, w, opts...)
}

type nopWriteCloser struct{ io.Writer }

func (wc *nopWriteCloser) Close() error { return nil }
