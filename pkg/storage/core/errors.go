package core

import (
	goerrors "errors"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var (
	// ErrNotImplemented can be returned for implementers that do not
	// implement a specific part of an interface.
	ErrNotImplemented = goerrors.New("not implemented")
	// ErrInvalidParameter specifies that a given parameter
	// (as a public struct field or function argument) was
	// not valid according to the specification.
	ErrInvalidParameter = goerrors.New("invalid parameter")
)

// StatusError is an error that supports also conversion
// to a metav1.Status struct for more detailed information.
type StatusError interface {
	error
	errors.APIStatus
}

func NewErrNotFound(id UnversionedObjectID) StatusError {
	return errors.NewNotFound(schema.GroupResource{
		Group:    id.GroupKind().Group,
		Resource: id.GroupKind().Kind,
	}, id.ObjectKey().Name)
}

func NewErrAlreadyExists(id UnversionedObjectID) StatusError {
	return errors.NewAlreadyExists(schema.GroupResource{
		Group:    id.GroupKind().Group,
		Resource: id.GroupKind().Kind,
	}, id.ObjectKey().Name)
}

func NewErrInvalid(id UnversionedObjectID, errs field.ErrorList) StatusError {
	return errors.NewInvalid(id.GroupKind(), id.ObjectKey().Name, errs)
}

var (
	IsErrNotFound      = errors.IsNotFound
	IsErrAlreadyExists = errors.IsAlreadyExists
	IsErrInvalid       = errors.IsInvalid
)
