package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

var (
	// ErrNoSuchNamespace means that the set of namespaces was searched in the
	// system, but the requested namespace wasn't in that list.
	ErrNoSuchNamespace = errors.New("no such namespace in the system")
)

// NamespaceEnforcer enforces a namespace policy for the Backend.
type NamespaceEnforcer interface {
	// EnforceNamespace makes sure that:
	// a) Any namespaced object has a non-empty namespace field after this call
	// b) Any non-namespaced object has an empty namespace field after this call
	// c) The applicable namespace policy of the user's liking is enforced (e.g.
	//    that there are only certain valid namespaces that can be used).
	//
	// This call is allowed to mutate obj. gvk represents the GroupVersionKind
	// of obj. The namespacer can be used to figure out if the given object is
	// namespaced or not. The given lister might be used to list object IDs,
	// or existing namespaces in the system.
	//
	// See GenericNamespaceEnforcer for an example implementation, or
	// pkg/storage/kube.NewNamespaceEnforcer() for a sample application.
	EnforceNamespace(ctx context.Context, obj Object, gvk core.GroupVersionKind, namespacer storage.Namespacer, lister storage.Lister) error
}

// GenericNamespaceEnforcer is a NamespaceEnforcer that:
// a) sets a default namespace for namespaced objects that have
//    the namespace field left empty
// b) makes sure non-namespaced objects do not have the namespace
//    field set, by pruning any previously-set value.
// c) if NamespaceGroupKind is non-nil; lists valid Namespace objects
//    in the system (of the given GroupKind); and matches namespaced
//    objects' namespace field against the listed Namespace objects'
//    .metadata.name field.
//
// For an example of how to configure this enforcer in the way
// Kubernetes itself (approximately) does, see pkg/storage/kube.
// NewNamespaceEnforcer().
type GenericNamespaceEnforcer struct {
	// DefaultNamespace describes the default namespace string
	// that should be set, if a namespaced object's namespace
	// field is empty.
	// +required
	DefaultNamespace string
	// NamespaceGroupKind describes the GroupKind for Namespace
	// objects in the system. If non-nil, objects with such
	// GroupKind are listed, and their .metadata.name is matched
	// against the current object's namespace field. If nil, any
	// namespace value is considered valid.
	// +optional
	NamespaceGroupKind *core.GroupKind
}

func (e GenericNamespaceEnforcer) EnforceNamespace(ctx context.Context, obj Object, gvk core.GroupVersionKind, namespacer storage.Namespacer, lister storage.Lister) error {
	// Get namespacing info
	namespaced, err := namespacer.IsNamespaced(gvk.GroupKind())
	if err != nil {
		return err
	}

	// Enforce generic rules
	ns := obj.GetNamespace()
	if !namespaced {
		// If a namespace was set, it must be sanitized, as non-namespaced
		// resources must have namespace field empty.
		if len(ns) != 0 {
			obj.SetNamespace("")
		}
		return nil
	}
	// The resource is namespaced.
	// If it is empty, set it to the default namespace.
	if len(ns) == 0 {
		// Verify that DefaultNamespace is non-empty
		if len(e.DefaultNamespace) == 0 {
			return fmt.Errorf("GenericNamespaceEnforcer.DefaultNamespace is mandatory: %w", core.ErrInvalidParameter)
		}
		// Mutate obj and set the namespace field to the default, then return
		obj.SetNamespace(e.DefaultNamespace)
		return nil
	}

	// If the namespace field is set, but NamespaceGroupKind is
	// nil, it means that any non-empty namespace value is
	// valid.
	if e.NamespaceGroupKind == nil {
		return nil
	}

	// However, if a Namespace GroupKind was given, look it up using
	// the lister, and verify its .metadata.name matches the given
	// namespace value.
	objIDs, err := lister.ListObjectIDs(ctx, *e.NamespaceGroupKind, "")
	if err != nil {
		return err
	}
	// Loop through the IDs, and try to match it against the set ns
	for _, id := range objIDs.List() {
		if id.ObjectKey().Name == ns {
			// Found the namespace; this is a valid setting
			return nil
		}
	}
	// The set namespace doesn't belong to the set of valid namespaces, error
	return fmt.Errorf("%w: %q", ErrNoSuchNamespace, ns)
}
