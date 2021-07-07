package frame

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// newHighlevelReader takes a "low-level" Reader (like *streamingReader or *yamlReader),
// and implements higher-level logic like proper closing, mutex locking and tracing.
func newHighlevelReader(r Reader, hasCloser bool, opts *ReaderOptions) Reader {
	return &highlevelReader{
		read:           r,
		res:            newClosableResource(r, hasCloser, *opts.CloseOnError, &opts.Tracer),
		opts:           opts,
		maxTotalFrames: opts.MaxFrames * 10,
	}
}

// highlevelReader uses the closableResource for the mutex locking, properly handling
// the close logic, and initiating the trace spans. On top of that it records extra
// tracing context in ReadFrame.
type highlevelReader struct {
	read Reader
	res  closableResource
	opts *ReaderOptions
	// successfulFrameCount counts the amount of successful frames read
	successfulFrameCount int64
	totalFrameCount      int64
	maxTotalFrames       int64
}

func (r *highlevelReader) ReadFrame(ctx context.Context) ([]byte, error) {
	var frame []byte
	err := r.res.accessResource(ctx, "ReadFrame", func(ctx context.Context, span trace.Span) error {

		// Refuse to write more than the maximum amount of successful frames
		if r.successfulFrameCount > r.opts.MaxFrames {
			return MakeErrFrameCountOverflowor(r.opts.MaxFrames)
		}

		// Call the underlying Reader. This MUST be done within r.res.accessResource in order to
		// be thread-safe.
		var err error
		frame, err = r.readFrame(ctx)
		if err != nil {
			return err
		}

		// Record how large the frame is
		spanAssociateFrameBytes(span, len(frame))
		return nil
	}, handleIoError)
	// handleIoError registers io.EOF as an "event", and other errors as "unknown errors" in the trace
	if err != nil {
		return nil, err
	}
	return frame, nil
}

func (r *highlevelReader) readFrame(ctx context.Context) ([]byte, error) {
	// Ensure the total number of frames doesn't overflow
	if r.totalFrameCount >= r.maxTotalFrames {
		return nil, MakeErrFrameCountOverflowor(r.opts.MaxFrames)
	}
	// Read the frame, and increase the total frame counter is increased
	frame, err := r.read.ReadFrame(ctx)
	r.totalFrameCount += 1
	if err != nil {
		return nil, err
	}

	// Sanitize the frame.
	frame, err = r.opts.Sanitizer.Sanitize(r.ContentType(), frame)
	if err != nil {
		return nil, err
	}

	// If it's empty, read the next frame automatically
	if len(frame) == 0 {
		return r.readFrame(ctx)
	}

	// Otherwise, if it's non-empty, return it and increase the "successful" counter
	r.successfulFrameCount += 1
	// If the frame count now overflows, return a ErrFrameCountOverflowor
	if r.successfulFrameCount > r.opts.MaxFrames {
		return nil, MakeErrFrameCountOverflowor(r.opts.MaxFrames)
	}
	return frame, nil
}

func (r *highlevelReader) ContentType() ContentType        { return r.read.ContentType() }
func (r *highlevelReader) Close(ctx context.Context) error { return r.res.Close(ctx) }
