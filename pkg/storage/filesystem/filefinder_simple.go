package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/util/sets"
)

// NewSimpleStorage is a default opinionated constructor for a Storage
// using SimpleFileFinder as the FileFinder, and the local disk as target.
// If you need more advanced customizablility than provided here, you can compose
// the call to filesystem.NewGeneric yourself.
func NewSimpleStorage(dir string, namespacer storage.Namespacer, opts SimpleFileFinderOptions) (Storage, error) {
	fs := NewOSFilesystem(dir)
	fileFinder, err := NewSimpleFileFinder(fs, opts)
	if err != nil {
		return nil, err
	}
	// fileFinder and namespacer are validated by filesystem.NewGeneric.
	return NewGeneric(fileFinder, namespacer)
}

func NewSimpleFileFinder(fs Filesystem, opts SimpleFileFinderOptions) (*SimpleFileFinder, error) {
	if fs == nil {
		return nil, fmt.Errorf("NewSimpleFileFinder: fs is mandatory")
	}
	ct := serializer.ContentTypeJSON
	if len(opts.ContentType) != 0 {
		ct = opts.ContentType
	}
	resolver := DefaultFileExtensionResolver
	if opts.FileExtensionResolver != nil {
		resolver = opts.FileExtensionResolver
	}
	return &SimpleFileFinder{
		fs:           fs,
		opts:         opts,
		contentTyper: StaticContentTyper{ContentType: ct},
		resolver:     resolver,
	}, nil
}

// isObjectIDNamespaced returns true if the ID is of a namespaced GroupKind, and
// false if the GroupKind is non-namespaced. NOTE: This ONLY works for FileFinders
// where the Storage has made sure that the namespacing conventions are followed.
func isObjectIDNamespaced(id core.UnversionedObjectID) bool {
	return id.ObjectKey().Namespace != ""
}

var _ FileFinder = &SimpleFileFinder{}

// SimpleFileFinder is a FileFinder-compliant implementation that
// stores Objects on disk using a straightforward directory layout.
//
// The following directory layout is used:
// if DisableGroupDirectory == false && SubDirectoryFileName == "" {
//	<dir>/<group>/<kind>/<namespace>/<name>.<ext> 	if namespaced or
//	<dir>/<group>/<kind>/<name>.<ext> 				if non-namespaced
// }
// else if DisableGroupDirectory == false && SubDirectoryFileName == "foo" {
//	<dir>/<group>/<kind>/<namespace>/<name>/foo.<ext> 	if namespaced or
//	<dir>/<group>/<kind>/<name>/foo.<ext> 				if non-namespaced
// }
// else if DisableGroupDirectory == true && SubDirectoryFileName == "" {
//	<dir>/<kind>/<namespace>/<name>.<ext> 	if namespaced or
//	<dir>/<kind>/<name>.<ext> 				if non-namespaced
// }
// else if DisableGroupDirectory == true && SubDirectoryFileName == "foo" {
//	<dir>/<kind>/<namespace>/<name>/foo.<ext> 	if namespaced or
//	<dir>/<kind>/<name>/foo.<ext> 				if non-namespaced
// }
//
// <ext> is resolved by the FileExtensionResolver, for the given ContentType.
// If <group> is an empty string (as when "apiVersion: v1" is used); <group> will
// be set to "core".
//
// This FileFinder does not support the ObjectAt method.
type SimpleFileFinder struct {
	fs           Filesystem
	opts         SimpleFileFinderOptions
	contentTyper StaticContentTyper
	resolver     FileExtensionResolver
}

type SimpleFileFinderOptions struct {
	// Default: false; means enable group directory
	DisableGroupDirectory bool
	// Default: ""; means use file names as the means of storage
	SubDirectoryFileName string
	// Default: serializer.ContentTypeJSON
	ContentType serializer.ContentType
	// Default: DefaultFileExtensionResolver
	FileExtensionResolver FileExtensionResolver
}

func (f *SimpleFileFinder) Filesystem() Filesystem {
	return f.fs
}

func (f *SimpleFileFinder) ContentTyper() ContentTyper {
	return f.contentTyper
}

// ObjectPath gets the file path relative to the root directory
func (f *SimpleFileFinder) ObjectPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// /<kindpath>/
	paths := []string{f.kindKeyPath(id.GroupKind())}

	if isObjectIDNamespaced(id) {
		// ./<namespace>/
		paths = append(paths, id.ObjectKey().Namespace)
	}
	// Get the file extension
	ext, err := f.ext()
	if err != nil {
		return "", err
	}
	if f.opts.SubDirectoryFileName == "" {
		// ./<name>.<ext>
		paths = append(paths, id.ObjectKey().Name+ext)
	} else {
		// ./<name>/<SubDirectoryFileName>.<ext>
		paths = append(paths, id.ObjectKey().Name, f.opts.SubDirectoryFileName+ext)
	}
	return filepath.Join(paths...), nil
}

func (f *SimpleFileFinder) kindKeyPath(gk core.GroupKind) string {
	if f.opts.DisableGroupDirectory {
		// ./<kind>/
		return filepath.Join(gk.Kind)
	}
	// Fall back to the "core/v1" storage path for "apiVersion: v1"
	group := gk.Group
	if len(group) == 0 {
		group = "core"
	}
	// ./<group>/<kind>/
	return filepath.Join(group, gk.Kind)
}

// ObjectsAt retrieves the ObjectIDs in the file with the given relative file path.
func (f *SimpleFileFinder) ObjectsAt(ctx context.Context, path string) (core.UnversionedObjectIDSet, error) {
	return nil, core.ErrNotImplemented
}

func (f *SimpleFileFinder) ext() (string, error) {
	return f.resolver.ExtensionForContentType(f.contentTyper.ContentType)
}

// ListGroupKinds returns all known GroupKinds by the implementation at that
// time. The set might vary over time as data is created and deleted; and
// should not be treated as an universal "what types could possibly exist",
// but more generally, "what are the GroupKinds of the objects that currently
// exist"? However, obviously, specific implementations might honor this
// guideline differently. This might be used for introspection into the system.
func (f *SimpleFileFinder) ListGroupKinds(ctx context.Context) ([]core.GroupKind, error) {
	if f.opts.DisableGroupDirectory {
		return nil, fmt.Errorf("cannot resolve GroupKinds when group directories are disabled: %w", core.ErrInvalidParameter)
	}

	// List groups at top-level
	groups, err := readDir(ctx, f.fs, "")
	if err != nil {
		return nil, err
	}
	// For all groups; also list all kinds, and add to the following list
	groupKinds := []core.GroupKind{}
	for _, group := range groups {
		kinds, err := readDir(ctx, f.fs, group)
		if err != nil {
			return nil, err
		}
		for _, kind := range kinds {
			groupKinds = append(groupKinds, core.GroupKind{Group: group, Kind: kind})
		}
	}
	return groupKinds, nil
}

// ListNamespaces lists the available namespaces for the given GroupKind.
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects. If any of the given rules are violated, ErrNamespacedMismatch
// should be returned as a wrapped error.
//
// The implementer can choose between basing the answer strictly on e.g.
// v1.Namespace objects that exist in the system, or just the set of
// different namespaces that have been set on any object belonging to
// the given GroupKind.
func (f *SimpleFileFinder) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	entries, err := readDir(ctx, f.fs, f.kindKeyPath(gk))
	if err != nil {
		return nil, err
	}
	return sets.NewString(entries...), nil
}

// ListObjectIDs returns a list of unversioned ObjectIDs.
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object IDs for that given namespace. If any of the given
// rules are violated, ErrNamespacedMismatch should be returned as a wrapped error.
func (f *SimpleFileFinder) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) (core.UnversionedObjectIDSet, error) {
	// If namespace is empty, the names will be in ./<kindkey>, otherwise ./<kindkey>/<ns>
	namesDir := filepath.Join(f.kindKeyPath(gk), namespace)
	entries, err := readDir(ctx, f.fs, namesDir)
	if err != nil {
		return nil, err
	}
	// Get the file extension
	ext, err := f.ext()
	if err != nil {
		return nil, err
	}
	// Map the names to UnversionedObjectIDs. We already know how many entries.
	ids := core.NewUnversionedObjectIDSetSized(len(entries))
	for _, entry := range entries {
		// Loop through all entries, and make sure they are sanitized .metadata.name's
		if f.opts.SubDirectoryFileName != "" {
			// If f.SubDirectoryFileName != "", the file names already match .metadata.name
			// Make sure the metadata file ./<.metadata.name>/<SubDirectoryFileName>.<ext> actually exists
			expectedPath := filepath.Join(namesDir, entry, f.opts.SubDirectoryFileName+ext)
			if exists, _ := f.fs.Exists(ctx, expectedPath); !exists {
				continue
			}
		} else {
			// Storage path is ./<name>.<ext>. entry is "<name>.<ext>"
			// Verify the extension is there and strip it from name. If ext isn't there, just continue
			if !strings.HasSuffix(entry, ext) {
				continue
			}
			// Remove the extension from the name
			entry = strings.TrimSuffix(entry, ext)
		}
		// If we got this far, add the key to the list
		ids.Insert(core.NewUnversionedObjectID(gk, core.ObjectKey{Name: entry, Namespace: namespace}))
	}
	return ids, nil
}

func readDir(ctx context.Context, fs Filesystem, dir string) ([]string, error) {
	fi, err := fs.Stat(ctx, dir)
	if os.IsNotExist(err) {
		// It's ok if the directory doesn't exist (yet), we just don't have any items then :)
		return nil, nil
	} else if !fi.IsDir() {
		// Unexpected, if the directory actually would be a file
		return nil, fmt.Errorf("expected that %s is a directory", dir)
	}

	// When we know that path is a directory, go ahead and read it
	entries, err := fs.ReadDir(ctx, dir)
	if err != nil {
		return nil, err
	}
	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		fileNames = append(fileNames, entry.Name())
	}
	return fileNames, nil
}
