package frame

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	customErr             = errors.New("custom")
	customErrIoReadCloser = errIoReadCloser(customErr)
)

func TestNewReader_Unrecognized(t *testing.T) {
	fr := NewReader(ContentType("doesnotexist"), customErrIoReadCloser)
	ctx := context.Background()
	frame, err := fr.ReadFrame(ctx)
	assert.ErrorIs(t, err, ErrUnsupportedContentType)
	assert.Len(t, frame, 0)
}

func Test_toReadCloser(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "toReadCloser.txt"))
	require.Nil(t, err)
	defer f.Close()

	tests := []struct {
		name          string
		r             io.Reader
		wantHasCloser bool
	}{
		{
			name:          "*bytes.Reader",
			r:             bytes.NewReader([]byte("foo")),
			wantHasCloser: false,
		},
		{
			name:          "*os.File",
			r:             f,
			wantHasCloser: true,
		},
		{
			name:          "os.Stdout",
			r:             os.Stdout,
			wantHasCloser: false,
		},
		{
			name:          "",
			r:             errIoReadCloser(nil),
			wantHasCloser: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRc, gotHasCloser := toReadCloser(tt.r)
			wantRc, _ := tt.r.(io.ReadCloser)
			if !tt.wantHasCloser {
				wantRc = io.NopCloser(tt.r)
			}
			assert.Equal(t, wantRc, gotRc)
			assert.Equal(t, tt.wantHasCloser, gotHasCloser)
		})
	}
}
