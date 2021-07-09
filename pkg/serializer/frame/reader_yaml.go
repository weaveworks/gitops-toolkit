package frame

import (
	"bufio"
	"context"
	"io"

	"k8s.io/apimachinery/pkg/util/yaml"
)

// newYAMLReader creates a "low-level" YAML Reader from the given io.ReadCloser.
func newYAMLReader(rc io.ReadCloser, opts *ReaderOptions) Reader {
	// Default maxFrameSize, to be sure
	maxFrameSize := defaultMaxFrameSize(opts.MaxFrameSize)
	// Limit the amount of bytes that can be read from the underlying io.ReadCloser.
	// Allow reading 4 bytes more than the maximum frame size, because the "---\n"
	// also counts for the IoLimitedReader.
	lr := NewIoLimitedReader(rc, maxFrameSize+4)
	// Construct the YAMLReader using a *bufio.Reader and the IoLimitedReader.
	return &yamlReader{
		r:             yaml.NewYAMLReader(bufio.NewReader(lr)),
		closer:        rc,
		limitedReader: lr,
		maxFrameSize:  maxFrameSize,
	}
}

// yamlReader is capable of returning individual YAML documents from the underlying io.ReadCloser.
// The returned YAML documents are sanitized such that they are non-empty, doesn't contain any
// leading or trailing "---" strings or whitespace (including newlines). There is always a trailing
// newline, however. The returned frame byte count <= opts.MaxFrameSize.
//
// Note: This Reader is a so-called "low-level" one. It doesn't do tracing, mutex locking, or
// proper closing logic. It must be wrapped by a composite, high-level Reader like highlevelReader.
type yamlReader struct {
	r             *yaml.YAMLReader
	limitedReader IoLimitedReader
	closer        io.Closer
	maxFrameSize  int64
}

func (r *yamlReader) ReadFrame(context.Context) ([]byte, error) {
	// Read one YAML document from the underlying YAMLReader. The YAMLReader reads the file line-by-line
	// using a *bufio.Reader. The *bufio.Reader reads in turn from an IoLimitedReader, which limits the
	// amount of bytes that can be read to avoid an endless data attack (which the YAMLReader doesn't
	// protect against). If the frame is too large, errors.Is(err, ErrFrameSizeOverflow) == true. Once
	// ErrFrameSizeOverflow has been returned once, it'll be returned for all consecutive calls (by design),
	// because the byte counter is never reset.
	frame, err := r.r.Read()
	if err != nil {
		return nil, err
	}

	// Reset now that we know a good frame has been read (err == nil)
	r.limitedReader.ResetCounter()

	// Enforce this "final" frame size <= maxFrameSize, as the limit on the IoLimitedReader was a bit less
	// restrictive (also allowed reading the YAML document separator).
	if int64(len(frame)) > r.maxFrameSize {
		return nil, MakeFrameSizeOverflowError(r.maxFrameSize)
	}

	return frame, nil
}

func (r *yamlReader) ContentType() ContentType    { return ContentTypeYAML }
func (r *yamlReader) Close(context.Context) error { return r.closer.Close() }
