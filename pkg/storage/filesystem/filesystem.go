package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/afero"
)

// Filesystem extends afero.Fs and afero.Afero with contexts added to every method.
type Filesystem interface {

	// Members of afero.Fs

	// MkdirAll creates a directory path and all parents that does not exist
	// yet.
	MkdirAll(ctx context.Context, path string, perm os.FileMode) error
	// Remove removes a file identified by name, returning an error, if any
	// happens.
	Remove(ctx context.Context, name string) error
	// Stat returns a FileInfo describing the named file, or an error, if any
	// happens.
	Stat(ctx context.Context, name string) (os.FileInfo, error)

	// Members of afero.Afero

	ReadDir(ctx context.Context, dirname string) ([]os.FileInfo, error)

	Exists(ctx context.Context, path string) (bool, error)

	ReadFile(ctx context.Context, filename string) ([]byte, error)

	WriteFile(ctx context.Context, filename string, data []byte, perm os.FileMode) error

	Walk(ctx context.Context, root string, walkFn filepath.WalkFunc) error

	// Custom methods

	// Checksum returns a checksum of the given file.
	//
	// What the checksum is is application-dependent, however, it
	// should be the same for two invocations, as long as the stored
	// data is the same. It might change over time although the
	// underlying data did not. Examples of checksums that can be
	// used is: the file modification timestamp, a sha256sum of the
	// file content, or the latest Git commit when the file was
	// changed.
	//
	// os.IsNotExist(err) can be used to check if the file doesn't
	// exist.
	Checksum(ctx context.Context, filename string) (string, error)

	// RootDirectory specifies where on disk the root directory is stored.
	// This path MUST be absolute. All other paths for the other methods
	// MUST be relative to this directory.
	RootDirectory() string
}

// NewOSFilesystem creates a new afero.OsFs for the local directory, using
// NewFilesystem underneath.
func NewOSFilesystem(rootDir string) Filesystem {
	return NewFilesystem(afero.NewOsFs(), rootDir)
}

// NewFilesystem wraps an underlying afero.Fs without context knowledge,
// in a Filesystem-compliant implementation; scoped at the given directory
// (i.e. wrapped in afero.NewBasePathFs(fs, rootDir)).
//
// Checksum is calculated based on the modification timestamp of the file.
func NewFilesystem(fs afero.Fs, rootDir string) Filesystem {
	// TODO: rootDir validation? It must be absolute, exist, and be a directory.
	return &filesystem{afero.NewBasePathFs(fs, rootDir), rootDir}
}

type filesystem struct {
	fs      afero.Fs
	rootDir string
}

func (f *filesystem) RootDirectory() string {
	return f.rootDir
}

func (f *filesystem) Checksum(ctx context.Context, filename string) (string, error) {
	fi, err := f.Stat(ctx, filename)
	if err != nil {
		return "", err
	}
	return checksumFromFileInfo(fi), nil
}

func (f *filesystem) MkdirAll(_ context.Context, path string, perm os.FileMode) error {
	return f.fs.MkdirAll(path, perm)
}

func (f *filesystem) Remove(_ context.Context, name string) error {
	return f.fs.Remove(name)
}

func (f *filesystem) Stat(_ context.Context, name string) (os.FileInfo, error) {
	return f.fs.Stat(name)
}

func (f *filesystem) ReadDir(_ context.Context, dirname string) ([]os.FileInfo, error) {
	return afero.ReadDir(f.fs, dirname)
}

func (f *filesystem) Exists(_ context.Context, path string) (bool, error) {
	return afero.Exists(f.fs, path)
}

func (f *filesystem) ReadFile(_ context.Context, filename string) ([]byte, error) {
	return afero.ReadFile(f.fs, filename)
}

func (f *filesystem) WriteFile(_ context.Context, filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(f.fs, filename, data, perm)
}

func (f *filesystem) Walk(_ context.Context, root string, walkFn filepath.WalkFunc) error {
	return afero.Walk(f.fs, root, walkFn)
}

func checksumFromFileInfo(fi os.FileInfo) string {
	return strconv.FormatInt(fi.ModTime().UnixNano(), 10)
}
