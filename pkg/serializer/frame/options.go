package frame

import (
	"github.com/weaveworks/libgitops/pkg/tracing"
	"k8s.io/utils/pointer"
)

// DefaultMaxFrames specifies the default maximum of frames that can be read or written by a Reader or Writer.
const DefaultMaxFrames = 1024

//
//	Reader Options
//

// guarantee that all options have non-nil, sensible defaults here
func defaultReaderOpts() *ReaderOptions {
	return &ReaderOptions{
		ReaderWriterOptions: ReaderWriterOptions{
			MaxFrameSize: DefaultMaxFrameSize,
			MaxFrames:    DefaultMaxFrames,
			Sanitizer:    DefaultSanitizer{},
			Tracer: tracing.TracerOptions{
				Name:      readerPrefix,
				UseGlobal: pointer.BoolPtr(true),
			},
		},
	}
}

// ReaderOption represents a functional apply of options to a Reader
type ReaderOption interface {
	// ApplyToReader applies mutations to the target struct
	ApplyToReader(target *ReaderOptions)
}

// ReaderOptions contains options for Readers.
type ReaderOptions struct {
	// ReaderOptions embeds all ReaderWriterOptions
	ReaderWriterOptions
}

// ReaderOptions implements ReaderOption.
var _ ReaderOption = &ReaderOptions{}

func (o *ReaderOptions) ApplyToReader(target *ReaderOptions) {
	o.ReaderWriterOptions.ApplyToReader(target)
}

// ApplyOptions applies all given mutators to o.
func (o *ReaderOptions) ApplyOptions(opts []ReaderOption) *ReaderOptions {
	for _, opt := range opts {
		opt.ApplyToReader(o)
	}
	return o
}

//
//	Writer Options
//

// guarantee that all options have non-nil, sensible defaults here
func defaultWriterOpts() *WriterOptions {
	return &WriterOptions{
		ReaderWriterOptions: ReaderWriterOptions{
			MaxFrameSize: DefaultMaxFrameSize,
			MaxFrames:    DefaultMaxFrames,
			Sanitizer:    DefaultSanitizer{},
			Tracer: tracing.TracerOptions{
				Name:      writerPrefix,
				UseGlobal: pointer.BoolPtr(true),
			},
		},
	}
}

// WriterOption represents a functional apply of options to a Writer
type WriterOption interface {
	// ApplyToWriter applies mutations to the target struct
	ApplyToWriter(target *WriterOptions)
}

// WriterOptions contains options for Writers.
type WriterOptions struct {
	// ReaderOptions embeds all ReaderWriterOptions
	ReaderWriterOptions
}

// WriterOptions implements WriterOption.
var _ WriterOption = &WriterOptions{}

func (o *WriterOptions) ApplyToWriter(target *WriterOptions) {
	o.ReaderWriterOptions.ApplyToWriter(target)
}

// ApplyOptions applies all given mutators to o.
func (o *WriterOptions) ApplyOptions(opts []WriterOption) *WriterOptions {
	for _, opt := range opts {
		opt.ApplyToWriter(o)
	}
	// it is guaranteed that all options are set, as defaultWriterOpts() includes all fields
	return o
}

//
//	Common Reader and Writer Options
//

// ReaderWriterOption is an union of ReaderOption and WriterOption,
// used as a return value for options that are applicable to both
// types of options.
type ReaderWriterOption interface {
	ReaderOption
	WriterOption
}

// ReaderWriterOptions contains options that are common to both Readers and Writers.
type ReaderWriterOptions struct {
	// MaxFrameSize specifies the maximum allowed frame size that can be read and returned.
	// Must be a positive integer. Defaults to DefaultMaxFrameSize.
	MaxFrameSize int64
	// MaxFrames specifies the maximum amount of successful frames that can be read or written
	// using a Reader or Writer. This means that e.g. empty frames after sanitation are NOT
	// counted as a frame in this context. When reading, there can be a maximum of 10*MaxFrames
	// in total (including failed and empty). Must be a positive integer. Defaults: DefaultMaxFrames.
	MaxFrames int64
	// Sanitizer configures the sanitizer that should be used for sanitizing the frames.
	Sanitizer Sanitizer
	// TracerOptions is embedded for reporting trace spans upstream
	Tracer tracing.TracerOptions
}

// ReaderWriterOptions implements the ReaderOption and WriterOption interfaces.
var _ ReaderWriterOption = &ReaderWriterOptions{}

// ApplyToCloser applies the option values in o to the target.
func (o *ReaderWriterOptions) applyCommon(target *ReaderWriterOptions) {
	if o.MaxFrameSize != 0 {
		target.MaxFrameSize = o.MaxFrameSize
	}
	if o.MaxFrames != 0 {
		target.MaxFrames = o.MaxFrames
	}
	o.Tracer.ApplyToTracer(&target.Tracer)
}

func (o *ReaderWriterOptions) ApplyToReader(target *ReaderOptions) {
	o.applyCommon(&target.ReaderWriterOptions)
}

func (o *ReaderWriterOptions) ApplyToWriter(target *WriterOptions) {
	o.applyCommon(&target.ReaderWriterOptions)
}
