package frame

import (
	"context"
	"errors"
	"io"

	"github.com/weaveworks/libgitops/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Constants shown in e.g. the Jaeger UI
	pkgPrefix    = "frame."
	readerPrefix = pkgPrefix + "Reader"
	writerPrefix = pkgPrefix + "Writer"
)

func spanAssociateFrameBytes(span trace.Span, len int) {
	span.SetAttributes(attribute.Int("frame_bytes", len))
}

func closeWithTrace(ctx context.Context, fnTracer tracing.FuncTracer, c Closer, hasCloser bool) tracing.TraceFuncResult {
	spanName := "Close"
	// Mark clearly that if hasCloser == false, no closing operation was performed
	if !hasCloser {
		spanName = "CloseNoop"
	}
	return fnTracer.TraceFunc(ctx, spanName, func(ctx context.Context, _ trace.Span) error {
		// Don't close if a closer wasn't given
		if !hasCloser {
			return nil
		}
		// Close the underlying resource
		return c.Close(ctx)
	})
}

// handleIoError registers io.EOF as an "event", and other errors as "unknown errors" in the trace
func handleIoError(span trace.Span, err error) error {
	// Register the error with the span. EOF is expected at some point,
	// hence, register that as an event instead of an error
	if errors.Is(err, io.EOF) {
		span.AddEvent("EOF")
	} else if err != nil {
		span.RecordError(err)
	}
	return err
}

// tracerOptionList is a helper type that applies the given TracerOptions to
// either a ReaderOption or WriterOption.
type tracerOptionList struct {
	opts []tracing.TracerOption
}

func (l tracerOptionList) ApplyToReader(target *ReaderOptions) {
	for _, opt := range l.opts {
		opt.ApplyToTracer(&target.Tracer)
	}
}

func (l tracerOptionList) ApplyToWriter(target *WriterOptions) {
	for _, opt := range l.opts {
		opt.ApplyToTracer(&target.Tracer)
	}
}

// WithTracerOptions transforms tracing.TracerOption implementations
// to ReaderOption and/or WriterOption-compatible options.
func WithTracerOptions(opts ...tracing.TracerOption) ReaderWriterOption {
	return tracerOptionList{opts}
}
