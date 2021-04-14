package core

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// Note: package core must not depend on any other parts of the libgitops repo, only
// essentially anything under k8s.io/apimachinery is ok.

// GroupVersionKind and ObjectID aliases
type GroupKind = schema.GroupKind
type GroupVersion = schema.GroupVersion
type GroupVersionKind = schema.GroupVersionKind
type ObjectKey = types.NamespacedName

// ObjectKeyFromObject returns the ObjectKey of a given metav1.Object.
func ObjectKeyFromMetav1Object(obj metav1.Object) ObjectKey {
	return ObjectKey{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}

// UnversionedObjectID represents an ID for an Object whose version is not known.
// However, the Group, Kind, Name and optionally, Namespace is known and should
// uniquely identify the Object at a specific moment in time.
type UnversionedObjectID interface {
	GroupKind() GroupKind
	ObjectKey() ObjectKey

	WithVersion(version string) ObjectID
	String() string // Implements fmt.Stringer
}

// ObjectID is a superset of UnversionedObjectID, that also specifies an exact version.
type ObjectID interface {
	UnversionedObjectID

	// WithoutVersion unwraps the underlying UnversionedObjectID; so it can
	// be used for e.g. equality (e.g. as a map key) operations.
	WithoutVersion() UnversionedObjectID
	GroupVersionKind() GroupVersionKind
}

// VersionRef is an interface that describes a reference to a specific version (for now; branch)
// of Objects in a Storage or Client.
type VersionRef interface {
	// Branch returns the branch name.
	Branch() string
	// IsZeroValue determines if this VersionRef is the "zero value", which means
	// that the caller should figure out how to handle that the user did not
	// give specific opinions of what version of the Object to get.
	IsZeroValue() bool
}
