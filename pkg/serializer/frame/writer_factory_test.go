package frame

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_toWriteCloser(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "toWriteCloser.txt"))
	require.Nil(t, err)
	defer f.Close()

	tests := []struct {
		name          string
		w             io.Writer
		wantHasCloser bool
	}{
		{
			name:          "*bytes.Buffer",
			w:             bytes.NewBuffer([]byte("foo")),
			wantHasCloser: false,
		},
		{
			name:          "*os.File",
			w:             f,
			wantHasCloser: true,
		},
		{
			name:          "os.Stdout",
			w:             os.Stdout,
			wantHasCloser: false,
		},
		{
			name:          "sample writecloser",
			w:             &ioWriteCloser{},
			wantHasCloser: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRc, gotHasCloser := toWriteCloser(tt.w)
			wantRc, _ := tt.w.(io.WriteCloser)
			if !tt.wantHasCloser {
				wantRc = &nopWriteCloser{tt.w}
			}
			assert.Equal(t, wantRc, gotRc)
			assert.Equal(t, tt.wantHasCloser, gotHasCloser)
		})
	}
}
