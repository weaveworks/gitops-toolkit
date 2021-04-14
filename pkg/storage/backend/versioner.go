package backend

import (
	"fmt"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/runtime"
)

// StorageVersioner is an interface that determines what version the Object
// with the given ID should be serialized as.
type StorageVersioner interface {
	StorageVersion(id core.ObjectID) (core.GroupVersion, error)
}

// SchemePreferredVersioner uses the prioritization information in the runtime.Scheme to
// determine what the preferred version should be. The caller is responsible for
// registering this information with the scheme using scheme.SetVersionPriority() before
// using this StorageVersioner. If SetVersionPriority has not been run, the version returned
// completely arbitrary.
type SchemePreferredVersioner struct {
	Scheme *runtime.Scheme
}

func (v SchemePreferredVersioner) StorageVersion(id core.ObjectID) (core.GroupVersion, error) {
	if v.Scheme == nil {
		return core.GroupVersion{}, fmt.Errorf("programmer error: SchemePreferredVersioner.Scheme must not be nil")
	}
	return serializer.PreferredVersionForGroup(v.Scheme, id.GroupKind().Group)
}
