package distributed

import (
	"context"
	"time"

	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
)

// Client is a client that can sync state with a remote in a transactional way.
//
// A distributed.Client is itself most likely both a CommitHook and TransactionHook; if so,
// it should be automatically registered with the transactional.Client's *HookChain in the
// distributed.Client's constructor.
type Client interface {
	// The distributed Client extends the transactional Client
	transactional.Client

	// StartResyncLoop starts a resync loop for the given branches for
	// the given interval.
	//
	// resyncCacheInterval specifies the interval for which resyncs
	// (remote Pulls) should be run in the background. The duration must
	// be positive, and non-zero.
	//
	// resyncBranches specifies what branches to resync. The default is
	// []string{""}, i.e. only the "default" branch.
	//
	// ctx should be used to cancel the loop, if needed.
	//
	// While it is technically possible to start many of these resync
	// loops, it is not recommended. Start it once, for all the branches
	// you need. The branches will be pulled synchronously in order. The
	// resync interval is non-sliding, which means that the interval
	// includes the time of the operations.
	StartResyncLoop(ctx context.Context, resyncCacheInterval time.Duration, resyncBranches ...string)

	// Remote exposes the underlying remote used
	Remote() Remote
}

type Remote interface {
	// Push pushes the attached branch (of the ctx) to the remote.
	// Push must block as long as the operation is in progress, but also
	// respect the timeout set on ctx and return instantly after it expires.
	//
	// It is guaranteed that Pull() and Push() are never called racily at
	// the same time for the same branch, BUT Pull() and Push() might be called
	// at the same time in any order for distinct branches. If the underlying
	// Remote transport only supports one "writer transport" to it at the same time,
	// the Remote must coordinate pulls and pushes with a mutex internally.
	Push(ctx context.Context) error

	// Pull pulls the attached branch (of the ctx) from the remote.
	// Pull must block as long as the operation is in progress, but also
	// respect the timeout set on ctx and return instantly after it expires.
	//
	// It is guaranteed that Pull() and Push() are never called racily at
	// the same time for the same branch, BUT Pull() and Push() might be called
	// at the same time in any order for distinct branches. If the underlying
	// Remote transport only supports one "writer transport" to it at the same time,
	// the Remote must coordinate pulls and pushes with a mutex internally.
	Pull(ctx context.Context) error
}

// LockableRemote describes a remote that supports locking a remote branch for writing.
type LockableRemote interface {
	Remote

	// Lock locks the branch attached to the context for writing, for the given duration.
	Lock(ctx context.Context, d time.Duration) error
	// Unlock reverses the write lock created by Lock()
	Unlock(ctx context.Context) error
}
