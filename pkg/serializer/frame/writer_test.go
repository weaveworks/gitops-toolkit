package frame

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewWriter_Unrecognized(t *testing.T) {
	fr := NewWriter(FramingType("doesnotexist"), ioutil.Discard)
	ctx := context.Background()
	err := fr.WriteFrame(ctx, make([]byte, 1))
	assert.ErrorIs(t, err, ErrUnsupportedFramingType)
}

func TestWriterShortBuffer(t *testing.T) {
	var buf bytes.Buffer
	w := &halfWriter{&buf}
	ctx := context.Background()
	err := NewYAMLWriter(w).WriteFrame(ctx, []byte("foo: bar"))
	assert.Equal(t, io.ErrShortWrite, err)
}

type halfWriter struct {
	w io.Writer
}

func (w *halfWriter) Write(p []byte) (int, error) {
	return w.w.Write(p[0 : (len(p)+1)/2])
}
