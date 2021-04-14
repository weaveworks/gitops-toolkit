package serializer

import (
	"io"
	"sync/atomic"
)

// NewSingleFrameReader returns a FrameReader for only a single frame of
// the specified content type. This avoids overhead if it is known that the
// byte array only contains one frame. The given frame is returned in
// whole in the first ReadFrame() call, and io.EOF is returned in all future
// invocations. This FrameReader works for any ContentType and transparently
// exposes the given content type through the ContentType() method.
// This implementation is thread-safe.
func NewSingleFrameReader(b []byte, ct ContentType) FrameReader {
	return &singleFrameReader{
		ct:          ct,
		b:           b,
		hasBeenRead: 0,
	}
}

// singleFrameReader implements the FrameReader interface.
var _ FrameReader = &singleFrameReader{}

type singleFrameReader struct {
	ct          ContentType
	b           []byte
	hasBeenRead uint32
}

func (r *singleFrameReader) ReadFrame() ([]byte, error) {
	// The first time this function executes; hasBeenRead == 0. The atomic compare-and-swap
	// operation checks if hasBeenRead == 0, and if so, sets it to one and returns true.
	// This means that r.b will ever only be returned exactly once, as all the other cases
	// (when hasBeenRead == 1), the compare-and-swap operation will return false => io.EOF.
	if atomic.CompareAndSwapUint32(&r.hasBeenRead, 0, 1) {
		// The first time, return the single frame we store
		return r.b, nil
	}
	return nil, io.EOF
}

func (r *singleFrameReader) ContentType() ContentType { return r.ct }
func (r *singleFrameReader) Close() error             { return nil }

// NewSingleFrameWriter returns a FrameWriter for only a single frame of
// the specified content type, using the underlying Writer. This FrameWriter
// will only ever write once; any successive calls will result in a io.ErrClosedPipe.
// This FrameWriter works for any ContentType and transparently exposes the given
// content type through the ContentType() method.
// This implementation is thread-safe.
func NewSingleFrameWriter(w Writer, ct ContentType) FrameWriter {
	return &singleFrameWriter{
		ct:             ct,
		w:              w,
		hasBeenWritten: 0,
	}
}

// singleFrameWriter implements the FrameWriter interface.
var _ FrameWriter = &singleFrameWriter{}

type singleFrameWriter struct {
	ct             ContentType
	w              Writer
	hasBeenWritten uint32
}

func (r *singleFrameWriter) Write(p []byte) (n int, err error) {
	// The first time this function executes; hasBeenWritten == 0. The atomic compare-and-swap
	// operation checks if hasBeenWritten == 0, and if so, sets it to one and returns true.
	// This means that r.b will ever only be returned exactly once, as all the other cases
	// (when hasBeenWritten == 1), the compare-and-swap operation will return false => io.ErrClosedPipe.
	if atomic.CompareAndSwapUint32(&r.hasBeenWritten, 0, 1) {
		// The first time, write to the underlying writer
		n, err = r.w.Write(p)
		return
	}
	err = io.ErrClosedPipe
	return
}

func (r *singleFrameWriter) ContentType() ContentType { return r.ct }
