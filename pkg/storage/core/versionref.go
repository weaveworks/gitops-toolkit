package core

import (
	"context"
)

var versionRefKey = versionRefKeyImpl{}

type versionRefKeyImpl struct{}

// WithVersionRef attaches the given VersionRef to a Context (it
// overwrites if one already exists in ctx). The key for the ref
// is private in this package, so one must use this function to
// register it.
func WithVersionRef(ctx context.Context, ref VersionRef) context.Context {
	return context.WithValue(ctx, versionRefKey, ref)
}

// GetVersionRef returns the VersionRef attached to this context.
// If there is no attached VersionRef, or it is nil, a BranchRef
// with branch "" will be returned as the "zero value" of VersionRef.
func GetVersionRef(ctx context.Context) VersionRef {
	r, ok := ctx.Value(versionRefKey).(VersionRef)
	// Return default ref if none specified
	if r == nil || !ok {
		return NewBranchRef("")
	}
	return r
}

// NewBranchRef creates a new VersionRef for a given branch. It is
// valid for the branch to be ""; in this case it means the "zero
// value", or unspecified branch to be more precise, where the caller
// can choose how to handle.
func NewBranchRef(branch string) VersionRef { return branchRef{branch} }

type branchRef struct{ branch string }

func (r branchRef) Branch() string { return r.branch }

// A branch is considered the zero value if the branch is an empty string,
// which it is e.g. when there was no VersionRef associated with a Context.
func (r branchRef) IsZeroValue() bool { return r.branch == "" }
