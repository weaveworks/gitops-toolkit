package kube

import (
	"sync"

	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TODO: Make an example component that iterates through all of a raw.Storage's
// or FileFinder's objects, and just reads them, converts them into the current
// hub version.

// TODO: Make a composite Storage that encrypts secrets using a key

// NewNamespaceEnforcer returns a backend.NamespaceEnforcer that
// enforces namespacing rules (approximately) in the same way as
// Kubernetes itself does. The following rules are applied:
//
// if object is namespaced {
// 		if .metadata.namespace == "" {
// 			.metadata.namespace = "default"
// 		} else { // .metadata.namespace != ""
// 			Make sure that such a v1.Namespace object
// 			exists in the system.
//		}
// } else { // object is non-namespaced
//		if .metadata.namespace != "" {
// 			.metadata.namespace = ""
//		}
// }
//
// Underneath, backend.GenericNamespaceEnforcer is used. Refer
// to the documentation of that if you want the functionality
// to be slightly different. (e.g. any namespace value is valid).
//
// TODO: Maybe we want to validate the namespace string itself?
func NewNamespaceEnforcer() backend.NamespaceEnforcer {
	return backend.GenericNamespaceEnforcer{
		DefaultNamespace: metav1.NamespaceDefault,
		NamespaceGroupKind: &core.GroupKind{
			Group: "", // legacy name for the core API group
			Kind:  "Namespace",
		},
	}
}

// SimpleRESTMapper is a subset of the meta.RESTMapper interface
type SimpleRESTMapper interface {
	// RESTMapping identifies a preferred resource mapping for the provided group kind.
	RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)
}

// RESTMapperToNamespacer implements the Namespacer interface by fetching (and caching) data
// from the given RESTMapper interface, that is compatible with any meta.RESTMapper implementation.
// This allows you to e.g. pass in a meta.RESTMapper yielded from
// sigs.k8s.io/controller-runtime/pkg/client/apiutil.NewDiscoveryRESTMapper(c *rest.Config), or
// k8s.io/client-go/restmapper.NewDiscoveryRESTMapper(groups []*restmapper.APIGroupResources)
// in order to look up namespacing information from either a running API server, or statically, from
// the list of restmapper.APIGroupResources.
func RESTMapperToNamespacer(mapper SimpleRESTMapper) storage.Namespacer {
	return &restNamespacer{
		mapper:        mapper,
		mappingByType: make(map[schema.GroupKind]*meta.RESTMapping),
		mu:            &sync.RWMutex{},
	}
}

var _ storage.Namespacer = &restNamespacer{}

type restNamespacer struct {
	mapper SimpleRESTMapper

	mappingByType map[schema.GroupKind]*meta.RESTMapping
	mu            *sync.RWMutex
}

func (n *restNamespacer) IsNamespaced(gk schema.GroupKind) (bool, error) {
	m, err := n.getMapping(gk)
	if err != nil {
		return false, err
	}
	return mappingNamespaced(m), nil
}

func (n *restNamespacer) getMapping(gk schema.GroupKind) (*meta.RESTMapping, error) {
	n.mu.RLock()
	mapping, ok := n.mappingByType[gk]
	n.mu.RUnlock()
	// If already cached, we're ok
	if ok {
		return mapping, nil
	}

	// Write the mapping info to our cache
	n.mu.Lock()
	defer n.mu.Unlock()
	m, err := n.mapper.RESTMapping(gk)
	if err != nil {
		return nil, err
	}
	n.mappingByType[gk] = m
	return m, nil
}

func mappingNamespaced(mapping *meta.RESTMapping) bool {
	return mapping.Scope.Name() == meta.RESTScopeNameNamespace
}
