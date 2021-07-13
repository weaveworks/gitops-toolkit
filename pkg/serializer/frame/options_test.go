package frame

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/weaveworks/libgitops/pkg/tracing"
	"go.opentelemetry.io/otel"
)

func compareOptions(t *testing.T, name string, got, want interface{}) {
	// We want to include the unexported tracer field when comparing TracerOptions, hence use reflect.DeepEqual
	// for the comparison
	opt := cmp.Comparer(func(x, y tracing.TracerOptions) bool {
		return reflect.DeepEqual(x, y)
	})
	// Report error with diff if not equal
	if !cmp.Equal(got, want, opt) {
		t.Errorf("%s: got vs want: %s", name, cmp.Diff(got, want, opt))
	}
}

func TestApplyReaderOptions(t *testing.T) {
	defaultWithMutation := func(apply func(*ReaderOptions)) *ReaderOptions {
		o := defaultReaderOpts()
		apply(o)
		return o
	}
	tests := []struct {
		name        string
		opts        []ReaderOption
		fromDefault bool
		want        *ReaderOptions
	}{
		{
			name:        "simple defaults",
			fromDefault: true,
			want:        defaultReaderOpts(),
		},
		{
			name: "MaxFrameSize: apply",
			opts: []ReaderOption{&ReaderWriterOptions{MaxFrameSize: 1234}},
			want: &ReaderOptions{ReaderWriterOptions: ReaderWriterOptions{MaxFrameSize: 1234}},
		},
		{
			name:        "MaxFrameSize: override default",
			opts:        []ReaderOption{&ReaderWriterOptions{MaxFrameSize: 1234}},
			fromDefault: true,
			want: defaultWithMutation(func(ro *ReaderOptions) {
				ro.MaxFrameSize = 1234
			}),
		},
		{
			name:        "MaxFrameSize: zero value has no effect",
			opts:        []ReaderOption{&ReaderWriterOptions{MaxFrameSize: 0}},
			fromDefault: true,
			want:        defaultReaderOpts(),
		},
		{
			name: "MaxFrameSize: latter overrides earlier, if set",
			opts: []ReaderOption{
				&ReaderWriterOptions{MaxFrameSize: 1234},
				&ReaderWriterOptions{MaxFrameSize: 4321},
				&ReaderWriterOptions{MaxFrameSize: 0},
			},
			want: &ReaderOptions{ReaderWriterOptions: ReaderWriterOptions{MaxFrameSize: 4321}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var from *ReaderOptions
			if tt.fromDefault {
				from = defaultReaderOpts()
			} else {
				from = &ReaderOptions{}
			}

			got := from.ApplyOptions(tt.opts)
			compareOptions(t, "TestApplyReaderOptions", got, tt.want)
		})
	}
}

func TestApplyReaderWriterOptions(t *testing.T) {
	defReadWithMutation := func(apply func(*ReaderOptions)) *ReaderOptions {
		o := defaultReaderOpts()
		apply(o)
		return o
	}
	defWriteWithMutation := func(apply func(*WriterOptions)) *WriterOptions {
		o := defaultWriterOpts()
		apply(o)
		return o
	}
	barTracer := otel.GetTracerProvider().Tracer("bar")
	tests := []struct {
		name        string
		opts        []ReaderWriterOption
		fromDefault bool
		wantReader  *ReaderOptions
		wantWriter  *WriterOptions
	}{
		{
			name:        "simple defaults",
			fromDefault: true,
			wantReader:  defaultReaderOpts(),
			wantWriter:  defaultWriterOpts(),
		},
		{
			name:        "WithTracerOptions: Set Tracer.Name",
			fromDefault: true,
			opts:        []ReaderWriterOption{WithTracerOptions(tracing.TracerOptions{Name: "foo"})},
			wantReader: defReadWithMutation(func(ro *ReaderOptions) {
				ro.Tracer.Name = "foo"
			}),
			wantWriter: defWriteWithMutation(func(wo *WriterOptions) {
				wo.Tracer.Name = "foo"
			}),
		},
		{
			name:        "WithTracerOptions: Set Tracer",
			fromDefault: true,
			opts:        []ReaderWriterOption{WithTracerOptions(tracing.WithTracer(barTracer))},
			wantReader: defReadWithMutation(func(ro *ReaderOptions) {
				// The tracer field is private, hence we need to configure it like this
				tracing.WithTracer(barTracer).ApplyToTracer(&ro.Tracer)
			}),
			wantWriter: defWriteWithMutation(func(wo *WriterOptions) {
				// The tracer field is private, hence we need to configure it like this
				tracing.WithTracer(barTracer).ApplyToTracer(&wo.Tracer)
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fromReader *ReaderOptions
			var fromWriter *WriterOptions
			if tt.fromDefault {
				fromReader = defaultReaderOpts()
				fromWriter = defaultWriterOpts()
			} else {
				fromReader = &ReaderOptions{}
				fromWriter = &WriterOptions{}
			}

			readOpts := []ReaderOption{}
			writeOpts := []WriterOption{}
			for _, opt := range tt.opts {
				readOpts = append(readOpts, opt)
				writeOpts = append(writeOpts, opt)
			}

			gotReader := fromReader.ApplyOptions(readOpts)
			gotWriter := fromWriter.ApplyOptions(writeOpts)
			compareOptions(t, "TestApplyReaderWriterOptions", gotReader, tt.wantReader)
			compareOptions(t, "TestApplyReaderWriterOptions", gotWriter, tt.wantWriter)
		})
	}
}
