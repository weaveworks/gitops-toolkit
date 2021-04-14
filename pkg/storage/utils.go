package storage

import (
	"fmt"

	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// VerifyNamespaced verifies that the given GroupKind and namespace parameter follows
// the rule of the Namespacer.
func VerifyNamespaced(namespacer Namespacer, gk core.GroupKind, ns string) error {
	// Get namespacing info
	namespaced, err := namespacer.IsNamespaced(gk)
	if err != nil {
		return err
	}
	if namespaced && ns == "" {
		return fmt.Errorf("%w: namespaced kind %v requires non-empty namespace", ErrNamespacedMismatch, gk)
	} else if !namespaced && ns != "" {
		return fmt.Errorf("%w: non-namespaced kind %v must not have namespace parameter set", ErrNamespacedMismatch, gk)
	}
	return nil
}
