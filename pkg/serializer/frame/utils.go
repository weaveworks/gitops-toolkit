package frame

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing/iotest"
)

// FrameList is a list of frames (byte arrays), used for convenience functions
type FrameList [][]byte

// ReadFrameList is a convenience method that reads all available frames from the Reader
// into a returned FrameList. If an error occurs, reading stops and the error is returned.
func ReadFrameList(ctx context.Context, fr Reader) (FrameList, error) {
	var frameList FrameList
	for {
		// Read until we get io.EOF or an error
		frame, err := fr.ReadFrame(ctx)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		// Append all frames to the returned list
		frameList = append(frameList, frame)
	}
	return frameList, nil
}

// WriteFrameList is a convenience method that writes a set of frames to a Writer.
// If an error occurs, writing stops and the error is returned.
func WriteFrameList(ctx context.Context, fw Writer, frameList FrameList) error {
	// Loop all frames in the list, and write them individually to the Writer
	for _, frame := range frameList {
		if err := fw.WriteFrame(ctx, frame); err != nil {
			return err
		}
	}
	return nil
}

// FromFile returns an io.ReadCloser from the given file, or an io.ReadCloser which returns
// the given file open error when read.
func FromFile(filePath string) io.ReadCloser {
	f, err := os.Open(filePath)
	if err != nil {
		return errIoReadCloser(err)
	}
	return f
}

// FromBytes returns an io.Reader from the given byte content.
func FromBytes(content []byte) io.Reader {
	return bytes.NewReader(content)
}

// FromString returns an io.Reader from the given string content.
func FromString(content string) io.Reader {
	return strings.NewReader(content)
}

func errIoReadCloser(err error) io.ReadCloser { return ioutil.NopCloser(iotest.ErrReader(err)) }

func isStdio(s interface{}) bool {
	f, ok := s.(*os.File)
	if !ok {
		return false
	}
	return int(f.Fd()) < 3
}

// ToIoWriteCloser transforms a Writer to an io.WriteCloser, by binding a relevant
// context.Context to it. If err != nil, then n == 0. If err == nil, then n == len(frame).
func ToIoWriteCloser(ctx context.Context, w Writer) io.WriteCloser {
	return &ioWriterHelper{ctx, w}
}

type ioWriterHelper struct {
	ctx    context.Context
	parent Writer
}

func (w *ioWriterHelper) Write(frame []byte) (n int, err error) {
	if err := w.parent.WriteFrame(w.ctx, frame); err != nil {
		return 0, err
	}
	return len(frame), nil
}
func (w *ioWriterHelper) Close() error {
	return w.parent.Close(w.ctx)
}
