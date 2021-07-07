package tracing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/utils/pointer"
)

func TestTracerOptions_getTracer(t *testing.T) {
	tests := []struct {
		name   string
		global trace.TracerProvider
		opts   []TracerOption
		want   trace.Tracer
	}{
		{
			name: "empty",
			opts: []TracerOption{TracerOptions{}},
			want: trace.NewNoopTracerProvider().Tracer(""),
		},
		{
			name: "with name",
			opts: []TracerOption{TracerOptions{Name: "foo"}},
			want: trace.NewNoopTracerProvider().Tracer("foo"),
		},
		{
			name:   "use global",
			global: customTp{},
			opts:   []TracerOption{TracerOptions{Name: "foo", UseGlobal: pointer.BoolPtr(true)}},
			want:   trace.NewNoopTracerProvider().Tracer("custom-foo"),
		},
		{
			name:   "use global",
			global: customTp{},
			opts:   []TracerOption{TracerOptions{Name: "foo", UseGlobal: pointer.BoolPtr(true)}},
			want:   trace.NewNoopTracerProvider().Tracer("custom-foo"),
		},
		{
			name: "use custom tp",
			opts: []TracerOption{TracerOptions{Name: "foo"}, WithTracerProvider(customTp{})},
			want: trace.NewNoopTracerProvider().Tracer("custom-foo"),
		},
		{
			name: "use custom tracer",
			opts: []TracerOption{TracerOptions{Name: "foo"}, WithTracer(customTp{}.Tracer("custom-bar"))},
			want: customTp{}.Tracer("custom-bar"),
		},
	}
	for _, tt := range tests {
		earlierTp := otel.GetTracerProvider()
		if tt.global != nil {
			otel.SetTracerProvider(tt.global)
		}
		o := TracerOptions{}
		for _, opt := range tt.opts {
			opt.ApplyToTracer(&o)
		}
		got := o.getTracer()
		assert.Equal(t, tt.want, got)
		if tt.global != nil {
			otel.SetTracerProvider(earlierTp)
		}
	}
}

type customTp struct{}

func (customTp) Tracer(instrumentationName string, opts ...trace.TracerOption) trace.Tracer {
	return trace.NewNoopTracerProvider().Tracer("custom-" + instrumentationName)
}
