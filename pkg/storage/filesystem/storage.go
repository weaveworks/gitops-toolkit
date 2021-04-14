package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/util/sets"
)

// NewGeneric creates a new Generic using the given lower-level
// FileFinder and Namespacer.
func NewGeneric(fileFinder FileFinder, namespacer storage.Namespacer) (Storage, error) {
	if fileFinder == nil {
		return nil, fmt.Errorf("NewGeneric: fileFinder is mandatory")
	}
	if namespacer == nil {
		return nil, fmt.Errorf("NewGeneric: namespacer is mandatory")
	}

	return &Generic{
		fileFinder: fileFinder,
		namespacer: namespacer,
	}, nil
}

// Generic is a Storage-compliant implementation, that
// combines the given lower-level FileFinder, Namespacer and Filesystem interfaces
// in a generic manner.
type Generic struct {
	fileFinder FileFinder
	namespacer storage.Namespacer
}

func (r *Generic) Namespacer() storage.Namespacer {
	return r.namespacer
}

func (r *Generic) FileFinder() FileFinder {
	return r.fileFinder
}

func (r *Generic) Read(ctx context.Context, id core.UnversionedObjectID) ([]byte, error) {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return nil, err
	}
	// Check if the resource indicated by key exists
	if !r.exists(ctx, p) {
		return nil, core.NewErrNotFound(id)
	}
	// Read the file
	return r.FileFinder().Filesystem().ReadFile(ctx, p)
}

func (r *Generic) Exists(ctx context.Context, id core.UnversionedObjectID) bool {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return false
	}
	return r.exists(ctx, p)
}

func (r *Generic) exists(ctx context.Context, path string) bool {
	exists, _ := r.FileFinder().Filesystem().Exists(ctx, path)
	return exists
}

func (r *Generic) Checksum(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return "", err
	}
	// Return a "high level" error if the file does not exist
	checksum, err := r.FileFinder().Filesystem().Checksum(ctx, p)
	if os.IsNotExist(err) {
		return "", core.NewErrNotFound(id)
	} else if err != nil {
		return "", err
	}
	return checksum, nil
}

func (r *Generic) ContentType(ctx context.Context, id core.UnversionedObjectID) (serializer.ContentType, error) {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return "", err
	}
	return r.FileFinder().ContentTyper().ContentTypeForPath(ctx, r.fileFinder.Filesystem(), p)
}

func (r *Generic) Write(ctx context.Context, id core.UnversionedObjectID, content []byte) error {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return err
	}

	// Create the underlying directories if they do not exist already
	if !r.exists(ctx, p) {
		if err := r.FileFinder().Filesystem().MkdirAll(ctx, filepath.Dir(p), 0755); err != nil {
			return err
		}
	}
	// Write the file content
	return r.FileFinder().Filesystem().WriteFile(ctx, p, content, 0664)
}

func (r *Generic) Delete(ctx context.Context, id core.UnversionedObjectID) error {
	// Get the path and verify namespacing info
	p, err := r.getPath(ctx, id)
	if err != nil {
		return err
	}

	// Check if the resource indicated by key exists
	if !r.exists(ctx, p) {
		return core.NewErrNotFound(id)
	}
	// Remove the file
	return r.FileFinder().Filesystem().Remove(ctx, p)
}

// ListGroupKinds returns all known GroupKinds by the implementation at that
// time. The set might vary over time as data is created and deleted; and
// should not be treated as an universal "what types could possibly exist",
// but more generally, "what are the GroupKinds of the objects that currently
// exist"? However, obviously, specific implementations might honor this
// guideline differently. This might be used for introspection into the system.
func (r *Generic) ListGroupKinds(ctx context.Context) ([]core.GroupKind, error) {
	// Just use the underlying filefinder
	return r.FileFinder().ListGroupKinds(ctx)
}

// ListNamespaces lists the available namespaces for the given GroupKind.
// This function shall only be called for namespaced objects, it is up to
// the caller to make sure they do not call this method for root-spaced
// objects; for that the behavior is undefined (but returning an error
// is recommended).
func (r *Generic) ListNamespaces(ctx context.Context, gk core.GroupKind) (sets.String, error) {
	namespaced, err := r.namespacer.IsNamespaced(gk)
	if err != nil {
		return nil, err
	}
	// Validate the groupkind
	if !namespaced {
		return nil, fmt.Errorf("%w: cannot list namespaces for non-namespaced kind: %v", storage.ErrNamespacedMismatch, gk)
	}
	// Just use the underlying filefinder
	return r.FileFinder().ListNamespaces(ctx, gk)
}

// ListObjectIDs returns a list of unversioned ObjectIDs.
// For namespaced GroupKinds, the caller must provide a namespace, and for
// root-spaced GroupKinds, the caller must not. When namespaced, this function
// must only return object IDs for that given namespace.
func (r *Generic) ListObjectIDs(ctx context.Context, gk core.GroupKind, namespace string) (core.UnversionedObjectIDSet, error) {
	// Validate the namespace parameter
	if err := storage.VerifyNamespaced(r.Namespacer(), gk, namespace); err != nil {
		return nil, err
	}
	// Just use the underlying filefinder
	return r.FileFinder().ListObjectIDs(ctx, gk, namespace)
}

func (r *Generic) getPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Verify namespacing info
	if err := storage.VerifyNamespaced(r.Namespacer(), id.GroupKind(), id.ObjectKey().Namespace); err != nil {
		return "", err
	}
	// Get the path
	return r.FileFinder().ObjectPath(ctx, id)
}
