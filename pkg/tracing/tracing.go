package tracing

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
)

// TODO: Make a Composite tracer that mocks the span such that logs are output as soon as
// the span is changed (or when it's ended) => logr or some similar backend
// or a SpanProcessor that outputs logs based on incoming traces
// TODO: Make a SpanProcessor that can output relevant YAML based on what's happening, for
// unit testing.

// TracerOption is a functional interface for TracerOptions
type TracerOption interface {
	ApplyToTracer(target *TracerOptions)
}

// FuncTracer is a higher-level type than the core trace.Tracer, which allows instrumenting
// a function running in a closure. It'll automatically create a span with the given name
// (plus maybe a pre-configured prefix). TraceFunc also returns a TraceFuncResult which allows
// the error to be instrumented automatically as well.
type FuncTracer interface {
	trace.Tracer
	// TraceFunc creates a trace with the given name while fn is executing.
	// ErrFuncNotSupplied is returned if fn is nil.
	TraceFunc(ctx context.Context, spanName string, fn TraceFunc, opts ...trace.SpanStartOption) TraceFuncResult
}

// TraceFuncResult can either just simply return the error from TraceFunc, or register the error using
// DefaultErrRegisterFunc (and then return it), or register the error using a custom error handling function.
type TraceFuncResult interface {
	// Error returns the error without any registration of it to the span.
	Error() error
	// Register registers the error using DefaultErrRegisterFunc.
	Register() error
	// RegisterCustom registers the error with the span using fn.
	// ErrFuncNotSupplied is returned if fn is nil.
	RegisterCustom(fn ErrRegisterFunc) error
}

// ErrFuncNotSupplied is raised when a supplied function callback is nil.
var ErrFuncNotSupplied = errors.New("function argument not supplied")

// MakeFuncNotSuppliedError formats ErrFuncNotSupplied in a standard way.
func MakeFuncNotSuppliedError(name string) error {
	return fmt.Errorf("%w: %s", ErrFuncNotSupplied, name)
}

// TraceFunc represents an instrumented function closure.
type TraceFunc func(context.Context, trace.Span) error

// ErrRegisterFunc should register the return error of TraceFunc err with the span
// using some logic, and then return err.
type ErrRegisterFunc func(span trace.Span, err error) error

// TracerOptions implements TracerOption, trace.Tracer and FuncTracer.
var _ TracerOption = TracerOptions{}
var _ trace.Tracer = TracerOptions{}
var _ FuncTracer = TracerOptions{}

// TracerOptions contains options for creating a trace.Tracer and FuncTracer.
type TracerOptions struct {
	// Name, if set to a non-empty value, will serve as the prefix for spans generated
	// using the FuncTracer as "{o.Name}.{spanName}" (otherwise just "{spanName}"), and
	// as the name of the trace.Tracer.
	Name string
	// UseGlobal specifies to default to the global tracing provider if true
	// (or, just use a no-op TracerProvider, if false). This only applies to if neither
	// WithTracer or WithTracerProvider have been supplied.
	UseGlobal *bool
	// provider is what TracerProvider to use for creating a tracer. If nil,
	// trace.NewNoopTracerProvider() is used.
	provider trace.TracerProvider
	// tracer can be set to use a specific tracer in Start(). If nil, a
	// tracer is created using the provider.
	tracer trace.Tracer
}

func (o TracerOptions) ApplyToTracer(target *TracerOptions) {
	if len(o.Name) != 0 {
		target.Name = o.Name
	}
	if o.UseGlobal != nil {
		target.UseGlobal = o.UseGlobal
	}
	if o.provider != nil {
		target.provider = o.provider
	}
	if o.tracer != nil {
		target.tracer = o.tracer
	}
}

// SpanName appends the name of the given function (spanName) to the given
// o.Name, if set. The return value of this function is aimed to be
// the name of the span, which will then be of the form "{o.Name}.{spanName}",
// or just "{spanName}".
func (o TracerOptions) fmtSpanName(spanName string) string {
	if len(o.Name) == 0 {
		return spanName
	}
	return o.Name + "." + spanName
}

func (o TracerOptions) tracerProvider() trace.TracerProvider {
	if o.provider != nil {
		return o.provider
	} else if o.UseGlobal != nil && *o.UseGlobal {
		return otel.GetTracerProvider()
	} else {
		return trace.NewNoopTracerProvider()
	}
}

func (o TracerOptions) getTracer() trace.Tracer {
	if o.tracer != nil {
		return o.tracer
	} else {
		return o.tracerProvider().Tracer(o.Name)
	}
}

func (o TracerOptions) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return o.getTracer().Start(ctx, o.fmtSpanName(spanName), opts...)
}

func (o TracerOptions) TraceFunc(ctx context.Context, spanName string, fn TraceFunc, opts ...trace.SpanStartOption) TraceFuncResult {
	ctx, span := o.Start(ctx, spanName, opts...)
	defer span.End()

	// Catch if fn == nil
	if fn == nil {
		return &traceFuncResult{MakeFuncNotSuppliedError("FuncTracer.TraceFunc"), span}
	}

	return &traceFuncResult{fn(ctx, span), span}
}

type traceFuncResult struct {
	err  error
	span trace.Span
}

func (r *traceFuncResult) Error() error {
	return r.err
}

func (r *traceFuncResult) Register() error {
	return r.RegisterCustom(DefaultErrRegisterFunc)
}

func (r *traceFuncResult) RegisterCustom(fn ErrRegisterFunc) error {
	// Catch the fn == nil case
	if fn == nil {
		err := multierr.Combine(r.err, MakeFuncNotSuppliedError("TraceFuncResult.RegisterCustom"))
		return DefaultErrRegisterFunc(r.span, err)
	}

	return fn(r.span, r.err)
}

// DefaultErrRegisterFunc registers the error with the span using span.RecordError(err)
// if the error is non-nil, and then returns the error unchanged.
func DefaultErrRegisterFunc(span trace.Span, err error) error {
	if err != nil {
		span.RecordError(err)
	}
	return err
}

// WithTracer returns a TracerOption which sets the tracer to TracerOptions explicitely.
func WithTracer(t trace.Tracer) TracerOption {
	return TracerOptions{tracer: t}
}

// WithTracerProvider returns a TracerOption which sets the tracing provider to TracerOptions.
// Unless WithTracer is also supplied, this TracerProvider will be used to create the tracer for
// TracerOptions.
func WithTracerProvider(tp trace.TracerProvider) TracerOption {
	return TracerOptions{provider: tp}
}
