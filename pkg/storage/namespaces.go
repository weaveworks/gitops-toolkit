package storage

import (
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// StaticNamespacer implements Namespacer
var _ Namespacer = StaticNamespacer{}

// StaticNamespacer has a default policy, which is that objects are in general namespaced
// (NamespacedIsDefaultPolicy == true), or that they are in general root-scoped
// (NamespacedIsDefaultPolicy == false).
//
// To the default policy, Exceptions can be added, so that for that GroupKind, the default
// policy is reversed.
type StaticNamespacer struct {
	NamespacedIsDefaultPolicy bool
	Exceptions                []core.GroupKind
}

func (n StaticNamespacer) IsNamespaced(gk core.GroupKind) (bool, error) {
	if n.NamespacedIsDefaultPolicy {
		// namespace by default, the gks list is a list of root-scoped entities
		return !n.gkIsException(gk), nil
	}
	// root by default, the gks in the list are namespaced
	return n.gkIsException(gk), nil
}

func (n StaticNamespacer) gkIsException(target core.GroupKind) bool {
	for _, gk := range n.Exceptions {
		if gk == target {
			return true
		}
	}
	return false
}
