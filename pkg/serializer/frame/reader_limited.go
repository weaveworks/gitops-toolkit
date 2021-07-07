package frame

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// DefaultMaxFrameSize is 16 MB, which matches the default behavior of Kubernetes.
const DefaultMaxFrameSize = 16 * 1024 * 1024

var (
	// FrameSizeOverflowErr is returned from Reader.ReadFrame or Writer.WriteFrame when a
	// frame exceeds the maximum allowed size.
	FrameSizeOverflowErr = errors.New("read frame was larger than maximum allowed size")
	// FrameCountOverflowErr is returned when a Reader or Writer have processed too many
	// frames.
	FrameCountOverflowErr = errors.New("the maximum amount of frames have been processed")
)

// MakeFrameSizeOverflowError returns a wrapped FrameSizeOverflowErr along with
// context in a normalized way.
func MakeFrameSizeOverflowError(maxFrameSize int64) error {
	return fmt.Errorf("%w %d bytes", FrameSizeOverflowErr, maxFrameSize)
}

// MakeFrameCountOverflowError returns a wrapped FrameCountOverflowErr along with
// context in a normalized way.
func MakeFrameCountOverflowError(maxFrames int64) error {
	return fmt.Errorf("%w: %d", FrameCountOverflowErr, maxFrames)
}

// IoLimitedReader is a specialized io.Reader helper type, which allows detecting when
// a read grows larger than the allowed maxFrameSize, returning a FrameSizeOverflowErr in that case.
//
// Internally there's a byte counter registering how many bytes have been read using the io.Reader
// across all Read calls since the last ResetCounter reset, which resets the byte counter to 0. This
// means that if you have successfully read one frame within bounds of maxFrameSize, and want to
// re-use the underlying io.Reader for the next frame, you shall run ResetCounter to start again.
//
// maxFrameSize is specified when constructing an IoLimitedReader, and defaults to DefaultMaxFrameSize.
//
// Note: The IoLimitedReader implementation is not thread-safe, that is for higher-level interfaces
// to implement and ensure.
type IoLimitedReader interface {
	// The byte count returned across consecutive Read(p) calls are at maximum maxFrameSize, until reset
	// by ResetCounter.
	io.Reader
	// ResetCounter resets the byte counter counting how many bytes have been read using Read(p)
	ResetCounter()
}

// NewIoLimitedReader makes a new IoLimitedReader implementation.
func NewIoLimitedReader(r io.Reader, maxFrameSize int64) IoLimitedReader {
	return &ioLimitedReader{
		reader:       r,
		buf:          new(bytes.Buffer),
		maxFrameSize: defaultMaxFrameSize(maxFrameSize),
	}
}

// defaultMaxFrameSize defaults maxFrameSize if unset
func defaultMaxFrameSize(maxFrameSize int64) int64 {
	// Default maxFrameSize if unset.
	if maxFrameSize == 0 {
		maxFrameSize = DefaultMaxFrameSize
	}
	return maxFrameSize
}

type ioLimitedReader struct {
	reader       io.Reader
	buf          *bytes.Buffer
	maxFrameSize int64
	frameBytes   int64
}

func (l *ioLimitedReader) Read(b []byte) (int, error) {
	// If we've already read more than we're allowed to, return an overflow error
	if l.frameBytes > l.maxFrameSize {
		// Keep returning this error as long as relevant
		return 0, MakeFrameSizeOverflowError(l.maxFrameSize)

	} else if l.frameBytes == l.maxFrameSize {
		// At this point we're not sure if the frame actually stops here or not
		// To figure that out; read one more byte into tmp
		tmp := make([]byte, 1)
		tmpn, err := l.reader.Read(tmp)

		// Write the read byte into the persistent buffer, for later use when l.frameBytes < l.maxFrameSize
		_, _ = l.buf.Write(tmp[:tmpn])
		// Increase the frameBytes, as bytes written to buf counts as "read"
		l.frameBytes += int64(tmpn)

		// Return the error from the read, if any. The most common error here is io.EOF.
		if err != nil {
			return 0, err
		}
		// Safeguard against a faulty reader. It's invalid to return tmpn == 0 and err == nil.
		if tmpn == 0 {
			return 0, io.ErrNoProgress
		}
		// Return that the frame overflowed now, as were able to read the byte (tmpn must be 1)
		return 0, MakeFrameSizeOverflowError(l.maxFrameSize)
	} // else l.frameBytes < l.maxFrameSize

	// We can at maximum read bytesLeft bytes more, shrink b accordingly if b is larger than the
	// maximum allowed amount to read.
	bytesLeft := l.maxFrameSize - l.frameBytes
	if int64(len(b)) > bytesLeft {
		b = b[:bytesLeft]
	}

	// First, flush any bytes in the buffer. By convention, the writes to buf have already
	// increased frameBytes, so no need to do that now. No need to check the error as buf
	// only returns io.EOF, and that's not important, it's even expected in most cases.
	m, _ := l.buf.Read(b)
	// Move the b slice forward m bytes as the m first bytes of b have now been populated
	b = b[m:]

	// Read from the reader into the rest of b
	n, err := l.reader.Read(b)
	// Register how many bytes have been read now additionally
	l.frameBytes += int64(n)
	return n, err
}

func (r *ioLimitedReader) ResetCounter() { r.frameBytes = 0 }
