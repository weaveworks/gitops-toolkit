package frame

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/trace"
)

func newHighlevelWriter(w Writer, hasCloser bool, opts *WriterOptions) Writer {
	return &highlevelWriter{
		writer:    w,
		writerMu:  &sync.Mutex{},
		hasCloser: hasCloser,
		opts:      opts,
	}
}

type highlevelWriter struct {
	writer   Writer
	writerMu *sync.Mutex

	hasCloser bool
	opts      *WriterOptions
	// frameCount counts the amount of successful frames written
	frameCount int64
}

func (w *highlevelWriter) WriteFrame(ctx context.Context, frame []byte) error {
	w.writerMu.Lock()
	defer w.writerMu.Unlock()

	return w.opts.Tracer.TraceFunc(ctx, "WriteFrame", func(ctx context.Context, span trace.Span) error {
		// Note: w.writer must only be accessed within a w.res.accessResource closure to ensure thread-safety

		// Refuse to write too large frames
		if int64(len(frame)) > w.opts.MaxFrameSize {
			return MakeFrameSizeOverflowError(w.opts.MaxFrameSize)
		}
		// Refuse to write more than the maximum amount of frames
		if w.frameCount >= w.opts.MaxFrames {
			return MakeFrameCountOverflowError(w.opts.MaxFrames)
		}

		// Sanitize the frame
		// TODO: Maybe create a composite writer that actually reads the given frame first, to
		// fully sanitize/validate it, and first then write the frames out using the writer?
		frame, err := w.opts.Sanitizer.Sanitize(w.FramingType(), frame)
		if err != nil {
			return err
		}

		// Catch empty frames
		if len(frame) == 0 {
			return nil
		}

		// Register the amount of (sanitized) bytes and call the underlying Writer
		spanAssociateFrameBytes(span, len(frame))
		err = w.writer.WriteFrame(ctx, frame)

		// Increase the frame counter, if the write was successful
		if err == nil {
			w.frameCount += 1
		}
		return err
	}).RegisterCustom(handleIoError)
	// handleIoError registers io.EOF as an "event", and other errors as "unknown errors" in the trace
}

func (w *highlevelWriter) FramingType() FramingType { return w.writer.FramingType() }
func (w *highlevelWriter) Close(ctx context.Context) error {
	return closeWithTrace(ctx, w.opts.Tracer, w.writer, w.hasCloser).Register()
}
