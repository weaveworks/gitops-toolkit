package filesystem

import (
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

// PathExcluder is an interface that lets the user implement custom policies
// for whether a given relative path to a given directory (fs is scoped at
// that directory) should be considered for an operation (e.g. inotify watch
// or file search).
type PathExcluder interface {
	// ShouldExcludePath takes in a relative path to the file which maybe
	// should be excluded.
	ShouldExcludePath(path string) bool
}

// DefaultPathExcluders returns a composition of
// ExcludeDirectoryNames{} for ".git" dirs and ExcludeExtensions{} for the ".swp" file extensions.
func DefaultPathExcluders() PathExcluder {
	return MultiPathExcluder{
		PathExcluders: []PathExcluder{
			ExcludeDirectoryNames{
				DirectoryNamesToExclude: []string{".git"},
			},
			ExcludeExtensions{
				Extensions: []string{".swp"}, // nano creates temporary .swp
			},
		},
	}
}

// ExcludeDirectoryNames implements PathExcluder.
var _ PathExcluder = ExcludeDirectoryNames{}

// ExcludeDirectories is a sample implementation of PathExcluder, that excludes
// files that have any parent directories with the given names.
type ExcludeDirectoryNames struct {
	DirectoryNamesToExclude []string
}

func (e ExcludeDirectoryNames) ShouldExcludePath(path string) bool {
	parts := strings.Split(filepath.Clean(path), string(os.PathSeparator))
	return sets.NewString(parts[:len(parts)-1]...).HasAny(e.DirectoryNamesToExclude...)
}

// ExcludeExtensions implements PathExcluder.
var _ PathExcluder = ExcludeExtensions{}

// ExcludeExtensions is a sample implementation of PathExcluder, that excludes
// all files with the given extensions. The strings in the Extensions slice
// must be in the form of filepath.Ext, i.e. ".json", ".txt", and so forth.
// The zero value of ExcludeExtensions excludes no files.
type ExcludeExtensions struct {
	Extensions []string
}

func (e ExcludeExtensions) ShouldExcludePath(path string) bool {
	ext := filepath.Ext(path)
	for _, exclExt := range e.Extensions {
		if ext == exclExt {
			return true
		}
	}
	return false
}

// MultiPathExcluder implements PathExcluder.
var _ PathExcluder = &MultiPathExcluder{}

// MultiPathExcluder is a composite PathExcluder that runs all of the
// PathExcluders in the slice one-by-one, and returns true if any of them
// does. The zero value of MultiPathExcluder excludes no files.
type MultiPathExcluder struct {
	PathExcluders []PathExcluder
}

func (m MultiPathExcluder) ShouldExcludePath(path string) bool {
	// Loop through all the excluders, and return true if any of them does
	for _, excl := range m.PathExcluders {
		if excl == nil {
			continue
		}
		if excl.ShouldExcludePath(path) {
			return true
		}
	}
	return false
}
