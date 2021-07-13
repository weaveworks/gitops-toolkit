package frame

import (
	"context"
	"io"
)

func newSingleReader(framingType FramingType, rc io.ReadCloser, o *ReaderOptions) Reader {
	return &singleReader{
		FramingTyped: framingType.ToFramingTyped(),
		r:            NewIoLimitedReader(rc, o.MaxFrameSize),
		c:            rc,
	}
}

func newSingleWriter(framingType FramingType, wc io.WriteCloser, _ *WriterOptions) Writer {
	return &singleWriter{
		FramingTyped: framingType.ToFramingTyped(),
		wc:           wc,
	}
}

// singleReader implements reading a single frame (up to a certain limit) from an io.ReadCloser.
// It MUST be wrapped in a higher-level composite Reader like the highlevelReader to satisfy the
// Reader interface correctly.
type singleReader struct {
	FramingTyped
	r           IoLimitedReader
	c           io.Closer
	hasBeenRead bool
}

// Read the whole frame from the underlying io.Reader, up to a given limit
func (r *singleReader) ReadFrame(context.Context) ([]byte, error) {
	// Only allow reading once
	if !r.hasBeenRead {
		// Read the whole frame from the underlying io.Reader, up to a given amount
		frame, err := io.ReadAll(r.r)
		// Mark we have read the frame
		r.hasBeenRead = true
		return frame, err
	}
	return nil, io.EOF
}

func (r *singleReader) Close(context.Context) error { return r.c.Close() }

// singleWriter implements writing a single frame to an io.WriteCloser.
// It MUST be wrapped in a higher-level composite Reader like the highlevelWriter to satisfy the
// Writer interface correctly.
type singleWriter struct {
	FramingTyped
	wc             io.WriteCloser
	hasBeenWritten bool
}

func (w *singleWriter) WriteFrame(_ context.Context, frame []byte) error {
	// Only allow writing once
	if !w.hasBeenWritten {
		// The first time, write the whole frame to the underlying writer
		n, err := w.wc.Write(frame)
		// Mark we have written the frame
		w.hasBeenWritten = true
		// Guard against short frames
		return catchShortWrite(n, err, frame)
	}
	// This really should never happen, because the higher-level Writer should ensure
	// that the frame is not ever written downstream if the maximum frame count is
	// exceeded, which it always is here as MaxFrames == 1 and w.hasBeenWritten == true.
	// In any case, for consistency, return io.ErrClosedPipe.
	return io.ErrClosedPipe
}

func (w *singleWriter) Close(context.Context) error { return w.wc.Close() }
