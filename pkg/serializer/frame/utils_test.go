package frame

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_isStdio(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "foo.txt"))
	require.Nil(t, err)
	defer f.Close()
	tests := []struct {
		name string
		in   interface{}
		want bool
	}{
		{
			name: "os.Stdin",
			in:   os.Stdin,
			want: true,
		},
		{
			name: "os.Stdout",
			in:   os.Stdout,
			want: true,
		},
		{
			name: "os.Stderr",
			in:   os.Stderr,
			want: true,
		},
		{
			name: "*bytes.Buffer",
			in:   bytes.NewBufferString("FooBar"),
		},
		{
			name: "*strings.Reader",
			in:   strings.NewReader("FooBar"),
		},
		{
			name: "*strings.Reader",
			in:   strings.NewReader("FooBar"),
		},
		{
			name: "*os.File",
			in:   f,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStdio(tt.in)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestFromConstructors(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "foo.yaml")
	str := "foo: bar\n"
	content := []byte(str)
	err := ioutil.WriteFile(yamlPath, content, 0644)
	require.Nil(t, err)

	ctx := context.Background()
	// FromFile -- found
	got, err := NewYAMLReader(FromFile(yamlPath)).ReadFrame(ctx)
	assert.Nil(t, err)
	assert.Equal(t, content, got)
	// FromFile -- not found
	got, err = NewYAMLReader(FromFile(filepath.Join(t.TempDir(), "notexist.yaml"))).ReadFrame(ctx)
	assert.NotNil(t, err)
	assert.Empty(t, got)
	// FromBytes
	got, err = NewYAMLReader(FromBytes(content)).ReadFrame(ctx)
	assert.Nil(t, err)
	assert.Equal(t, content, got)
	// FromString
	got, err = NewYAMLReader(FromString(str)).ReadFrame(ctx)
	assert.Nil(t, err)
	assert.Equal(t, str, string(got))
}

func TestToIoWriteCloser(t *testing.T) {
	var buf bytes.Buffer
	closeRec := &recordingCloser{}
	w := NewYAMLWriter(ioWriteCloser{&buf, closeRec})
	ctx := context.Background()
	iow := ToIoWriteCloser(ctx, w)

	content := []byte("foo: bar\n")
	n, err := iow.Write(content)
	assert.Len(t, content, n)
	assert.Nil(t, err)

	// Close the writer to provoke an error below
	assert.Nil(t, iow.Close())
	assert.Equal(t, 1, closeRec.count)

	// Try writing again
	n, err = iow.Write(content)
	assert.Equal(t, n, 0)
	assert.ErrorIs(t, err, io.ErrClosedPipe)
	// Assume the writer has been closed only once
	assert.Equal(t, 1, closeRec.count)
	assert.Equal(t, buf.String(), yamlSep+string(content))
}

func TestReadFrameList(t *testing.T) {
	r := NewYAMLReader(FromString(messyYAML))
	ctx := context.Background()
	// Happy case
	fr, err := ReadFrameList(ctx, r)
	assert.Equal(t, FrameList{[]byte(testYAML), []byte(testYAML)}, fr)
	assert.Nil(t, err)

	// Non-happy case
	r = NewJSONReader(FromString(messyJSON), &ReaderWriterOptions{MaxFrameSize: testJSONlen - 1})
	fr, err = ReadFrameList(ctx, r)
	assert.Len(t, fr, 0)
	assert.ErrorIs(t, err, ErrFrameSizeOverflow)
}

func TestWriteFrameList(t *testing.T) {
	var buf bytes.Buffer
	w := NewYAMLWriter(&buf)
	ctx := context.Background()
	// Happy case
	err := WriteFrameList(ctx, w, FrameList{[]byte(testYAML), []byte(testYAML)})
	assert.Equal(t, buf.String(), yamlSep+testYAML+yamlSep+testYAML)
	assert.Nil(t, err)

	// Non-happy case
	buf.Reset()
	w = NewJSONWriter(&buf, &ReaderWriterOptions{MaxFrameSize: testJSONlen})
	err = WriteFrameList(ctx, w, FrameList{[]byte(testJSON), []byte(testJSON2)})
	assert.Equal(t, buf.String(), testJSON)
	assert.ErrorIs(t, err, ErrFrameSizeOverflow)
}
