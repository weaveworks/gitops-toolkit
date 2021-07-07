package frame

import (
	"context"
	"io"
)

// Closer is like io.Closer, but with a Context passed along as well.
type Closer interface {
	// Close closes the underlying resource. The underlying io.Closer should be only
	// closed once.
	Close(ctx context.Context) error
}

// Reader is a content-type specific reader of an underlying io.Reader or io.ReadCloser.
// If an io.Reader is used, Close(ctx) is a no-op. If an io.ReadCloser is used, Close(ctx)
// will close the underlying io.ReadCloser.
//
// The Reader returns frames, as defined by the relevant content type.
// For example, for YAML a frame represents a YAML document, while JSON is a self-framing
// format, i.e. encoded objects can be written to a stream just as
// '{ "a": "" ... }{ "b": "" ... }' and separated from there.
//
// Another way of defining a "frame" is that it MUST contain exactly one decodable object.
// This means that no empty (i.e. len(frame) == 0) frames shall be returned. Note: The decodable
// object might represent a list object (e.g. as Kubernetes' v1.List); more generally something
// decodable into a Go struct.
//
// The Reader can use as many underlying Read(p []byte) (n int, err error) calls it needs
// to the underlying io.Read(Clos)er. As long as frames can successfully be read from the underlying
// io.Read(Clos)er, len(frame) != 0 and err == nil. When io.EOF is encountered, len(frame) == 0 and
// errors.Is(err, io.EOF) == true.
//
// The Reader MUST be thread-safe, i.e. it must use the underlying io.Read(Clos)er responsibly
// without causing race conditions, e.g. by guarding reads/closes with a mutual exclusion lock.
//
// Once the Reader has been closed (either directly using Close or indirectly by a previous error
// in ReadFrame, if ReadWriterOptions.CloseOnError == true), ReadFrame MUST return io.ErrClosedPipe.
// The Reader MUST directly abort the read operation if the frame size exceeds
// ReadWriterOptions.MaxFrameSize, and return FrameSizeOverflowErr.
//
// The Reader MUST return FrameCountOverflowErr if the underlying Reader has returned more than
// ReadWriterOptions.MaxFrames successful read operations. Returned errors (including io.EOF)
// MUST be checked for equality using errors.Is(err, target), NOT using err == target.
//
// The Reader MAY automatically close the underlying io.Read(Clos)er, depending on
// ReadWriterOptions.CloseOnError. The Reader MAY respect cancellation signals on the context,
// depending on ReaderOptions. The Reader MAY support reporting trace
// spans for how long certain operations take.
type Reader interface {
	// The Reader is specific to this content type
	ContentTyped
	// ReadFrame reads one frame from the underlying io.Read(Clos)er. At maximum, the frame is as
	// large as ReadWriterOptions.MaxFrameSize. See the documentation on the Reader interface for more
	// details.
	ReadFrame(ctx context.Context) ([]byte, error)
	// The Reader can be closed. If an underlying io.Reader is used, this is a no-op. If an
	// io.ReadCloser is used, this will close that io.ReadCloser.
	Closer
}

// ReaderFactory knows how to create various different Readers for
// given ContentTypes. ErrUnsupportedContentType MUST be returned if the given
// ContentType is not supported.
type ReaderFactory interface {
	// NewReader returns a new Reader for the given ContentType.
	// The options are parsed in order, and the latter options override the former.
	// The given io.Reader can also be a io.ReadCloser, and if so, Reader.Close(ctx)
	// will close that io.ReadCloser.
	// The ReaderFactory might allow any contentType as long as ReaderOptions.MaxFrames
	// is 1, because then there might not be a need to perform framing.
	NewReader(contentType ContentType, r io.Reader, opts ...ReaderOption) Reader
}

// Writer is a content-type specific writer to an underlying io.Writer or io.WriteCloser.
// If an io.Writer is used, Close(ctx) is a no-op. If an io.WriteCloser is used, Close(ctx)
// will close the underlying io.WriteCloser.
//
// The Writer writes frames to the underlying stream, as defined by the content type.
// For example, for YAML a frame represents a YAML document, while JSON is a self-framing
// format, i.e. encoded objects can be written to a stream just as
// '{ "a": "" ... }{ "b": "" ... }'.
//
// Another way of defining a "frame" is that it MUST contain exactly one decodable object.
// It is valid (but not recommended) to supply empty frames to the Writer.
//
// Writer will only call the underlying io.Write(Close)r's Write(p []byte) call once. If n < len(frame)
// and err == nil, io.ErrShortBuffer will be returned.
//
// The Writer MUST be thread-safe, i.e. it must use the underlying io.Write(Close)r responsibly
// without causing race conditions, e.g. by guarding writes/closes with a mutual exclusion lock.
//
// Once the Writer has been closed (either directly using Close or indirectly by a previous error
// in WriteFrame, if ReadWriterOptions.CloseOnError == true), WriteFrame MUST return io.ErrClosedPipe.
// Returned errors MUST be checked for equality using errors.Is(err, target), NOT using err == target.
//
// The Writer MUST directly abort the read operation if the frame size exceeds ReadWriterOptions.MaxFrameSize,
// and return FrameSizeOverflowErr. The Writer MUST ignore empty frames, where len(frame) == 0, possibly
// after sanitation. The Writer MUST return FrameCountOverflowErr if WriteFrame has been called more than
// ReadWriterOptions.MaxFrames times.
//
// The Writer MAY automatically close the underlying io.WriteCloser, depending on ReadWriterOptions.CloseOnError.
// The Writer MAY respect cancellation signals on the context, depending on WriterOptions. The Writer MAY
// support reporting trace spans for how long certain operations take.
type Writer interface {
	// The Reader is specific to this content type
	ContentTyped
	// WriteFrame writes one frame to the underlying io.Write(Close)r.
	// See the documentation on the Writer interface for more details.
	WriteFrame(ctx context.Context, frame []byte) error
	// The Writer can be closed. If an underlying io.Writer is used, this is a no-op. If an
	// io.WriteCloser is used, this will close that io.WriteCloser.
	Closer
}

// WriterFactory knows how to create various different Writers for
// given ContentTypes.
type WriterFactory interface {
	// NewWriter returns a new Writer for the given ContentType.
	// The options are parsed in order, and the latter options override the former.
	// The given io.Writer can also be a io.WriteCloser, and if so, Writer.Close(ctx)
	// will close that io.WriteCloser.
	// The WriterFactory might allow any contentType as long as WriterOptions.MaxFrames
	// is 1, because then there might not be a need to perform framing.
	NewWriter(contentType ContentType, w io.Writer, opts ...WriterOption) Writer
}

// Factory combines ReaderFactory and WriterFactory.
type Factory interface {
	ReaderFactory
	WriterFactory
}
