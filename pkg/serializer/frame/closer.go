package frame

import (
	"context"
	"io"
	"sync"

	"github.com/weaveworks/libgitops/pkg/tracing"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
)

// closableResource is a composite Closer which provides a mutex guarding an underlying (not specified here
// as Golang doesn't allow generics) resource. The closableResource enforces that accessResource and/or Close(ctx)
// is called, there is only one consumer of that resource at a time. The closableResource shall also set up tracing
// spans once the mutex is unlocked. If ReadWriterOptions.CloseOnError is true, the underlying resource will
// automatically be closed if an error occurs. The closableResource also enforces that if Close(ctx) has been called,
// the resource cannot be accessed anymore, instead io.ErrClosedPipe is returned.
type closableResource interface {
	Closer

	// accessResource lets the caller access (by convention) the closable resource in accessFn. fnName is
	// concatenated with the TracerOptions.Name and reported in the resulting span. errFn registers
	// the returned error with the span, for debugging visibility.
	accessResource(ctx context.Context, fnName string, accessFn tracing.TraceFunc, errFn tracing.ErrRegisterFunc) error
}

// newClosableResource returns a new closableResource on top of the given Closer. If hasCloser == true, the
// Closer will be used, if hasCloser == false, the Closer will not be called. This closableResource implements
// tracing and mutex capabilities.
func newClosableResource(c Closer, hasCloser, closeOnError bool, trOpts *tracing.TracerOptions) closableResource {
	return &closableResourceImpl{c, hasCloser, closeOnError, trOpts, &sync.Mutex{}, false}
}

type closableResourceImpl struct {
	c            Closer
	hasCloser    bool
	closeOnError bool
	trOpts       *tracing.TracerOptions
	mux          *sync.Mutex
	closed       bool
}

func (c *closableResourceImpl) accessResource(ctx context.Context, fnName string, accessFn tracing.TraceFunc, errFn tracing.ErrRegisterFunc) error {
	// Make sure we have access to the underlying resource
	c.mux.Lock()
	defer c.mux.Unlock()

	// Pipe the resulting error through the errFn to capture it to the telemetry platform (if provided)
	return c.trOpts.TraceFunc(ctx, fnName, func(ctx context.Context, span trace.Span) error {
		// If the resource already has been closed, return io.ErrClosedPipe
		if c.closed {
			return io.ErrClosedPipe
		}

		// Access the resource, and provide the span for registering additional data
		err := accessFn(ctx, span)
		// If an error was provided, and closing on errors is enabled automatically, close the resource
		if err != nil && c.closeOnError {
			// Important: At this point the mutex is locked from accessResource, hence, use the internal close function
			// that does not lock the mutex again.
			closeErr := c.close(ctx, errFn)
			// Combine the close error, if any.
			err = multierr.Combine(closeErr, err)
		}
		return err
	}).RegisterCustom(errFn)
}

func (c *closableResourceImpl) close(ctx context.Context, errFn tracing.ErrRegisterFunc) error {
	spanName := "Close"
	if !c.hasCloser { // Mark clearly that if hasCloser == false, no closing operation was performed
		spanName = "CloseNoop"
	}
	return c.trOpts.TraceFunc(ctx, spanName, func(ctx context.Context, span trace.Span) error {
		// Don't close the underlying resource twice
		if c.closed {
			return nil
		}
		// Always report io.ErrClosedPipe after close has been called
		c.closed = true
		// Do not report any extra traces, upstream has reported the resource MUST NOT be Closed
		// (e.g. in the case of stdin/stdout)
		if !c.hasCloser {
			return nil
		}
		// Close the underlying resource
		return c.c.Close(ctx)
	}).RegisterCustom(errFn)
}

func (c *closableResourceImpl) Close(ctx context.Context) error {
	// Make sure we have access to the underlying resource
	c.mux.Lock()
	defer c.mux.Unlock()

	return c.close(ctx, tracing.DefaultErrRegisterFunc)
}
