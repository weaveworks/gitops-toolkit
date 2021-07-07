package frame

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

func newHighlevelWriter(w Writer, hasCloser bool, opts *WriterOptions) Writer {
	return &highlevelWriter{
		writer: w,
		res:    newClosableResource(w, hasCloser, *opts.CloseOnError, &opts.Tracer),
		opts:   opts,
	}
}

type highlevelWriter struct {
	writer Writer
	res    closableResource
	opts   *WriterOptions
	// frameCount counts the amount of successful frames written
	frameCount int64
}

func (w *highlevelWriter) WriteFrame(ctx context.Context, frame []byte) error {
	return w.res.accessResource(ctx, "WriteFrame", func(ctx context.Context, span trace.Span) error {
		// Note: w.writer must only be accessed within a w.res.accessResource closure to ensure thread-safety

		// Refuse to write too large frames
		if int64(len(frame)) > w.opts.MaxFrameSize {
			return MakeErrFrameSizeOverflowor(w.opts.MaxFrameSize)
		}
		// Refuse to write more than the maximum amount of frames
		if w.frameCount >= w.opts.MaxFrames {
			return MakeErrFrameCountOverflowor(w.opts.MaxFrames)
		}

		// Sanitize the frame
		// TODO: Maybe create a composite writer that actually reads the given frame first, to
		// fully sanitize/validate it, and first then write the frames out using the writer?
		frame, err := w.opts.Sanitizer.Sanitize(w.ContentType(), frame)
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
	}, handleIoError)
	// handleIoError registers io.EOF as an "event", and other errors as "unknown errors" in the trace
}

func (w *highlevelWriter) ContentType() ContentType        { return w.writer.ContentType() }
func (w *highlevelWriter) Close(ctx context.Context) error { return w.res.Close(ctx) }
