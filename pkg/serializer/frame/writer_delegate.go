package frame

import (
	"context"
	"io"
)

func newDelegatingWriter(contentType ContentType, w io.Writer, c io.Closer, opts *WriterOptions) Writer {
	return &delegatingWriter{
		ContentTyped: contentType.ToContentTyped(),
		w:            w,
		c:            c,
		opts:         opts,
	}
}

// delegatingWriter is an implementation of the Writer interface
type delegatingWriter struct {
	ContentTyped
	w    io.Writer
	c    io.Closer
	opts *WriterOptions
}

func (w *delegatingWriter) WriteFrame(_ context.Context, frame []byte) error {
	// Write the frame to the underlying writer
	n, err := w.w.Write(frame)
	// Guard against short writes
	return catchShortWrite(n, err, frame)
}

func (w *delegatingWriter) Close(context.Context) error { return w.c.Close() }

func newErrWriter(contentType ContentType, err error) Writer {
	return &errWriter{contentType.ToContentTyped(), &nopCloser{}, err}
}

type errWriter struct {
	ContentTyped
	Closer
	err error
}

func (w *errWriter) WriteFrame(context.Context, []byte) error { return w.err }

func catchShortWrite(n int, err error, frame []byte) error {
	if n < len(frame) && err == nil {
		err = io.ErrShortWrite
	}
	return err
}
