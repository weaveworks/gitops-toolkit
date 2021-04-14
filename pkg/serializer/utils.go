package serializer

import (
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// LockedScheme describes a shared scheme that should be locked before writing, and unlocked
// after writing. Reading can be done safely without any locking.
type LockedScheme interface {
	Scheme() *runtime.Scheme
	SchemeLock()
	SchemeUnlock()
}

func newLockedScheme(scheme *runtime.Scheme) LockedScheme {
	return &lockedScheme{scheme, &sync.Mutex{}}
}

type lockedScheme struct {
	scheme *runtime.Scheme
	mu     *sync.Mutex
}

func (s *lockedScheme) Scheme() *runtime.Scheme {
	return s.scheme
}

func (s *lockedScheme) SchemeLock() {
	s.mu.Lock()
}

func (s *lockedScheme) SchemeUnlock() {
	s.mu.Unlock()
}

func GVKForObject(scheme *runtime.Scheme, obj runtime.Object) (schema.GroupVersionKind, error) {
	// Safety check: one should not do this
	if obj == nil || obj.GetObjectKind() == nil {
		return schema.GroupVersionKind{}, fmt.Errorf("GVKForObject: obj or obj.GetObjectKind() must not be nil")
	}

	// If this is a runtime.Unknown object, return the GVK stored in TypeMeta
	if gvk := obj.GetObjectKind().GroupVersionKind(); IsUnknown(obj) && !gvk.Empty() {
		return gvk, nil
	}

	// Special case: Allow objects with two versions to be registered, when the caller is specific
	// about what version they want populated.
	// This is needed essentially for working around that there are specific K8s types (structs)
	// that have been registered with multiple GVKs (e.g. a Deployment struct in both apps & extensions)
	// TODO: Maybe there is a better way to solve this? Remove unwanted entries from the scheme typeToGVK
	// map manually?
	gvks, _, _ := scheme.ObjectKinds(obj)
	if len(gvks) > 1 {
		// If we have a configuration with more than one gvk for the same object,
		// check the set GVK on the object to "choose" the right one, if exists in the list
		setGVK := obj.GetObjectKind().GroupVersionKind()
		if !setGVK.Empty() {
			for _, gvk := range gvks {
				if EqualsGVK(setGVK, gvk) {
					return gvk, nil
				}
			}
		}
	}

	// TODO: Should we just copy-paste this one, or move it into k8s core to avoid importing controller-runtime
	// only for this function?
	return apiutil.GVKForObject(obj, scheme)
}

// GVKForList returns the GroupVersionKind for the items in a given List type.
// In the case of Unstructured or PartialObjectMetadata, it is required that this
// information is already set in TypeMeta. The "List" suffix is never returned.
func GVKForList(obj client.ObjectList, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	// First, get the GVK as normal.
	gvk, err := GVKForObject(scheme, obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	// Make sure this is a list type, i.e. it has the an "Items" field.
	isList := meta.IsListType(obj)
	if !isList {
		return schema.GroupVersionKind{}, ErrObjectIsNotList
	}
	// Make sure the returned GVK never ends in List.
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")
	return gvk, nil
}

// PreferredVersionForGroup returns the most preferred version of a group in the scheme.
// In order to tell the scheme what your preferred ordering is, use scheme.SetVersionPriority().
func PreferredVersionForGroup(scheme *runtime.Scheme, groupName string) (schema.GroupVersion, error) {
	// Get the prioritized versions for the given group
	gvs := scheme.PrioritizedVersionsForGroup(groupName)
	if len(gvs) < 1 {
		return schema.GroupVersion{}, fmt.Errorf("expected some version to be registered for group %s", groupName)
	}
	// Use the first, preferred, (external) version
	return gvs[0], nil
}

// EqualsGK returns true if gk1 and gk2 have the same fields.
func EqualsGK(gk1, gk2 schema.GroupKind) bool {
	return gk1.Group == gk2.Group && gk1.Kind == gk2.Kind
}

// EqualsGVK returns true if gvk1 and gvk2 have the same fields.
func EqualsGVK(gvk1, gvk2 schema.GroupVersionKind) bool {
	return EqualsGK(gvk1.GroupKind(), gvk2.GroupKind()) && gvk1.Version == gvk2.Version
}

func IsUnknown(obj runtime.Object) bool {
	_, isUnknown := obj.(*runtime.Unknown)
	return isUnknown
}

func IsPartialObject(obj runtime.Object) bool {
	_, isPartial := obj.(*metav1.PartialObjectMetadata)
	return isPartial
}

func IsPartialObjectList(obj runtime.Object) bool {
	_, isPartialList := obj.(*metav1.PartialObjectMetadataList)
	return isPartialList
}

// IsUnstructured checks if obj is runtime.Unstructured
func IsUnstructured(obj runtime.Object) bool {
	_, isUnstructured := obj.(runtime.Unstructured)
	return isUnstructured
}

// IsUnstructuredList checks if obj is *unstructured.UnstructuredList
func IsUnstructuredList(obj runtime.Object) bool {
	_, isUnstructuredList := obj.(*unstructured.UnstructuredList)
	return isUnstructuredList
}

// IsNonConvertible returns true for unstructured, partial and unknown objects
// that should not be converted.
func IsNonConvertible(obj runtime.Object) bool {
	// TODO: Should Lists also be marked non-convertible?
	// IsUnstructured also covers IsUnstructuredList -- *UnstructuredList implements runtime.Unstructured
	return IsUnstructured(obj) || IsPartialObject(obj) || IsPartialObjectList(obj) || IsUnknown(obj)
}

// IsTyped returns true if the object is typed, i.e. registered with the given
// scheme and not unversioned.
func IsTyped(obj runtime.Object, scheme *runtime.Scheme) bool {
	_, isUnversioned, err := scheme.ObjectKinds(obj)
	return !isUnversioned && err == nil
}
