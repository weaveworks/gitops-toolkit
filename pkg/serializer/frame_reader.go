package serializer

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

const (
	defaultBufSize      = 64 * 1024        // 64 kB
	defaultMaxFrameSize = 16 * 1024 * 1024 // 16 MB
)

var (
	// FrameOverflowErr is returned from FrameReader.ReadFrame when one frame exceeds the
	// maximum size of 16 MB.
	FrameOverflowErr = errors.New("frame was larger than maximum allowed size")
)

// ReadCloser in this package is an alias for io.ReadCloser. It helps in Godoc to locate
// helpers in this package which returns writers (i.e. FromFile and FromBytes)
type ReadCloser io.ReadCloser

// FrameReader is a content-type specific reader of a given ReadCloser.
// The FrameReader reads frames from the underlying ReadCloser and returns them for consumption.
// When io.EOF is reached, the stream is closed automatically.
type FrameReader interface {
	ContentTyped
	io.Closer

	// ReadFrame reads frames from the underlying ReadCloser and returns them for consumption.
	// When io.EOF is reached, the stream is closed automatically.
	ReadFrame() ([]byte, error)
}

// FrameReaderFactory knows how to create various different FrameReaders for
// given ContentTypes.
type FrameReaderFactory interface {
	// NewFrameReader returns a new FrameReader for the given ContentType,
	// and ReadCloser that contains the underlying data that should be read.
	NewFrameReader(contentType ContentType, rc ReadCloser) FrameReader
}

// defaultFrameReaderFactory is the variable used in public methods.
var defaultFrameReaderFactory FrameReaderFactory = frameReaderFactory{}

// frameReaderFactory is the default implementation of FrameReaderFactory.
type frameReaderFactory struct{}

// Documentation below attached to NewFrameReader.
func (frameReaderFactory) NewFrameReader(contentType ContentType, rc ReadCloser) FrameReader {
	switch contentType {
	case ContentTypeYAML:
		return newFrameReader(json.YAMLFramer.NewFrameReader(rc), contentType)
	case ContentTypeJSON:
		return newFrameReader(json.Framer.NewFrameReader(rc), contentType)
	default:
		return &errFrameReader{ErrUnsupportedContentType, contentType}
	}
}

// NewFrameReaderFactory returns the default variant of FrameReaderFactory capable
// of creating YAML- and JSON-compatible FrameReaders.
func NewFrameReaderFactory() FrameReaderFactory {
	return frameReaderFactory{}
}

// NewFrameReader returns a FrameReader for the given ContentType and data in the
// ReadCloser. The Reader is automatically closed in io.EOF. ReadFrame is called
// once each Decoder.Decode() or Decoder.DecodeInto() call. When Decoder.DecodeAll() is
// called, the FrameReader is read until io.EOF, upon where it is closed.
func NewFrameReader(contentType ContentType, rc ReadCloser) FrameReader {
	return defaultFrameReaderFactory.NewFrameReader(contentType, rc)
}

// NewYAMLFrameReader returns a FrameReader that supports both YAML and JSON. Frames are separated by "---\n"
//
// This call is the same as NewFrameReader(ContentTypeYAML, rc)
func NewYAMLFrameReader(rc ReadCloser) FrameReader {
	return NewFrameReader(ContentTypeYAML, rc)
}

// NewJSONFrameReader returns a FrameReader that supports both JSON. Objects are read from the stream one-by-one,
// each object making up its own frame.
//
// This call is the same as NewFrameReader(ContentTypeJSON, rc)
func NewJSONFrameReader(rc ReadCloser) FrameReader {
	return NewFrameReader(ContentTypeJSON, rc)
}

// newFrameReader returns a new instance of the frameReader struct
func newFrameReader(rc io.ReadCloser, contentType ContentType) *frameReader {
	return &frameReader{
		rc:           rc,
		rcMu:         &sync.Mutex{},
		bufSize:      defaultBufSize,
		maxFrameSize: defaultMaxFrameSize,
		contentType:  contentType,
	}
}

// frameReader is a FrameReader implementation
type frameReader struct {
	// the underlying readcloser and the mutex that guards it
	rc   io.ReadCloser
	rcMu *sync.Mutex

	bufSize      int
	maxFrameSize int
	contentType  ContentType
}

// ReadFrame reads one frame from the underlying io.Reader. ReadFrame
// keeps on reading from the Reader in bufSize blocks, until the Reader either
// returns err == nil or EOF. If the Reader reports an ErrShortBuffer error,
// ReadFrame keeps on reading using new calls. ReadFrame might return both data and
// io.EOF. io.EOF will be returned in the final call.
func (rf *frameReader) ReadFrame() (frame []byte, err error) {
	// Only one actor can read at a time
	rf.rcMu.Lock()
	defer rf.rcMu.Unlock()

	// Temporary buffer to parts of a frame into
	var buf []byte
	// How many bytes were read by the read call
	var n int
	// Multiplier for bufsize
	c := 1
	for {
		// Allocate a buffer of a multiple of bufSize.
		buf = make([]byte, c*rf.bufSize)
		// Call the underlying reader.
		n, err = rf.rc.Read(buf)
		// Append the returned bytes to the b slice returned
		// If n is 0, this call is a no-op
		frame = append(frame, buf[:n]...)

		// If the frame got bigger than the max allowed size, return and report the error
		if len(frame) > rf.maxFrameSize {
			err = FrameOverflowErr
			return
		}

		// Handle different kinds of errors
		switch err {
		case io.ErrShortBuffer:
			// ignore the "buffer too short" error, and just keep on reading, now doubling the buffer
			c *= 2
			continue
		case nil:
			// One document is "done reading", we should return it if valid
			// Only return non-empty documents, i.e. skip e.g. leading `---`
			if len(bytes.TrimSpace(frame)) > 0 {
				// valid non-empty document
				return
			}
			// The document was empty, reset the frame (just to be sure) and continue
			frame = nil
			continue
		case io.EOF:
			// we reached the end of the file, close the reader and return
			rf.rc.Close()
			return
		default:
			// unknown error, return it immediately
			// TODO: Maybe return the error here?
			return
		}
	}
}

// ContentType returns the content type for the given FrameReader
func (rf *frameReader) ContentType() ContentType {
	return rf.contentType
}

// Close implements io.Closer and closes the underlying ReadCloser
func (rf *frameReader) Close() error {
	// Only one actor can access rf.rc at a time
	rf.rcMu.Lock()
	defer rf.rcMu.Unlock()

	return rf.rc.Close()
}

// FromFile returns a ReadCloser from the given file, or a ReadCloser which returns
// the given file open error when read.
func FromFile(filePath string) ReadCloser {
	f, err := os.Open(filePath)
	if err != nil {
		return &errReadCloser{err}
	}
	return f
}

// FromBytes returns a ReadCloser from the given byte content.
func FromBytes(content []byte) ReadCloser {
	return ioutil.NopCloser(bytes.NewReader(content))
}
