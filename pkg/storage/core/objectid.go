package core

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NewUnversionedObjectID creates a new UnversionedObjectID from the given GroupKind and ObjectKey.
func NewUnversionedObjectID(gk GroupKind, key ObjectKey) UnversionedObjectID {
	return unversionedObjectID{gk, key}
}

type unversionedObjectID struct {
	gk  GroupKind
	key ObjectKey
}

func (o unversionedObjectID) GroupKind() GroupKind                { return o.gk }
func (o unversionedObjectID) ObjectKey() ObjectKey                { return o.key }
func (o unversionedObjectID) WithVersion(version string) ObjectID { return objectID{o, version} }
func (o unversionedObjectID) String() string {
	if o.key.Namespace == "" {
		return fmt.Sprintf("UnversionedObjectID: groupkind=%s name=%s", o.gk, o.key.Name)
	}
	return fmt.Sprintf("UnversionedObjectID: groupkind=%s name=%s ns=%s", o.gk, o.key.Name, o.key.Namespace)
}

// NewObjectID creates a new ObjectID from the given GroupVersionKind and ObjectKey.
func NewObjectID(gvk GroupVersionKind, key ObjectKey) ObjectID {
	return objectID{unversionedObjectID{gvk.GroupKind(), key}, gvk.Version}
}

type objectID struct {
	unversionedObjectID
	version string
}

func (o objectID) WithoutVersion() UnversionedObjectID       { return o.unversionedObjectID }
func (o objectID) GroupVersionKind() schema.GroupVersionKind { return o.gk.WithVersion(o.version) }
func (o objectID) String() string {
	if o.key.Namespace == "" {
		return fmt.Sprintf("ObjectID: groupkind=%s version=%s name=%s", o.gk, o.version, o.key.Name)
	}
	return fmt.Sprintf("ObjectID: groupkind=%s version=%s name=%s ns=%s", o.gk, o.version, o.key.Name, o.key.Namespace)
}
