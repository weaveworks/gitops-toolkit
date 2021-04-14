package client

import (
	"errors"
	"fmt"

	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/runtime"
)

var ErrNoMetadata = errors.New("it is required to embed ObjectMeta into the serialized API type")

func NewObjectForGVK(gvk core.GroupVersionKind, scheme *runtime.Scheme) (Object, error) {
	kobj, err := scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	obj, ok := kobj.(Object)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoMetadata, gvk)
	}
	return obj, nil
}
