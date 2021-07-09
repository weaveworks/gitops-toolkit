package frame

import (
	"context"
	"errors"
	"io"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
)

// newJSONReader creates a "low-level" JSON Reader from the given io.ReadCloser.
func newJSONReader(rc io.ReadCloser, o *ReaderOptions) Reader {
	// json.Framer.NewFrameReader takes care of the actual JSON framing logic
	return newStreamingReader(FramingTypeJSON, json.Framer.NewFrameReader(rc), o)
}

// newStreamingReader makes a generic Reader that reads from an io.ReadCloser returned
// from Kubernetes' runtime.Framer.NewFrameReader, in exactly the way
// k8s.io/apimachinery/pkg/runtime/serializer/streaming implements this.
// On a high-level, it means that many small Read(p []byte) calls are made as long as
// io.ErrShortBuffer is returned. When err == nil is returned from rc, we know that we're
// at the end of a frame, and at that point the frame is returned.
//
// Note: This Reader is a so-called "low-level" one. It doesn't do tracing, mutex locking, or
// proper closing logic. It must be wrapped by a composite, high-level Reader like highlevelReader.
func newStreamingReader(contentType FramingType, rc io.ReadCloser, o *ReaderOptions) Reader {
	// Limit the amount of bytes that can be read in one frame
	lr := NewIoLimitedReader(rc, o.MaxFrameSize)
	return &streamingReader{
		FramingTyped: contentType.ToFramingTyped(),
		lr:           lr,
		streamReader: newK8sStreamingReader(ioReadCloser{lr, rc}, o.MaxFrameSize),
		maxFrameSize: o.MaxFrameSize,
	}
}

type ioReadCloser struct {
	io.Reader
	io.Closer
}

// streamingReader is a small "conversion" struct that implements the Reader interface for a
// given k8sStreamingReader. When reader_streaming_k8s.go is upstreamed, we can replace the
// temporary k8sStreamingReader interface with a "proper" Kubernetes one.
type streamingReader struct {
	FramingTyped
	lr           IoLimitedReader
	streamReader k8sStreamingReader
	maxFrameSize int64
}

func (r *streamingReader) ReadFrame(context.Context) ([]byte, error) {
	// Read one frame from the streamReader
	frame, err := r.streamReader.Read()
	if err != nil {
		// Transform streaming.ErrObjectTooLarge to a ErrFrameSizeOverflow, if returned.
		return nil, mapError(err, errorMappings{
			streaming.ErrObjectTooLarge: func() error {
				return MakeFrameSizeOverflowError(r.maxFrameSize)
			},
		})
	}
	// Reset the counter only when we have a successful frame
	r.lr.ResetCounter()
	return frame, nil
}

func (r *streamingReader) Close(context.Context) error { return r.streamReader.Close() }

// mapError is an utility for mapping a "actual" error to a lazily-evaluated "desired" one.
// Equality between the errorMappings' keys and err is defined by errors.Is
func mapError(err error, f errorMappings) error {
	for target, mkErr := range f {
		if errors.Is(err, target) {
			return mkErr()
		}
	}
	return err
}

// errorMappings maps actual errors to lazily-evaluated desired ones
type errorMappings map[error]mkErrorFunc

// mkErrorFunc lazily creates an error
type mkErrorFunc func() error
