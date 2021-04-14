package distributed

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/util/wait"
)

// NewClient creates a new distributed Client using the given underlying transactional Client,
// remote, and options that configure how the Client should respond to network partitions.
func NewClient(c transactional.Client, remote Remote, opts ...ClientOption) (*Generic, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: c is mandatory", core.ErrInvalidParameter)
	}
	if remote == nil {
		return nil, fmt.Errorf("%w: remote is mandatory", core.ErrInvalidParameter)
	}

	o := defaultOptions().ApplyOptions(opts)

	g := &Generic{
		Client:        c,
		remote:        remote,
		opts:          *o,
		branchLocks:   make(map[string]*branchLock),
		branchLocksMu: &sync.Mutex{},
	}

	// Register ourselves to hook into the transactional.Client's operations
	c.CommitHookChain().Register(g)
	c.TransactionHookChain().Register(g)

	return g, nil
}

type Generic struct {
	transactional.Client
	remote Remote
	opts   ClientOptions
	// branchLocks maps a given branch to a given lock the state of the branch
	branchLocks map[string]*branchLock
	// branchLocksMu guards branchLocks
	branchLocksMu *sync.Mutex
}

type branchLock struct {
	// mu should be write-locked whenever the branch is actively running any
	// function from the remote
	mu *sync.RWMutex
	// lastPull is guarded by mu, before reading, one should RLock mu
	lastPull time.Time
}

func (c *Generic) Get(ctx context.Context, key core.ObjectKey, obj client.Object) error {
	return c.readWhenPossible(ctx, func() error {
		return c.Client.Get(ctx, key, obj)
	})
}

func (c *Generic) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.readWhenPossible(ctx, func() error {
		return c.Client.List(ctx, list, opts...)
	})
}

func (c *Generic) readWhenPossible(ctx context.Context, operation func() error) error {
	branch := c.branchFromCtx(ctx)

	// Check if we need to do a pull before
	if c.needsResync(branch, c.opts.CacheValidDuration) {
		// Try to pull the remote branch. If it fails, use returnErr to figure out if
		// this (depending on the configured PACELC mode) is a critical error, or if we
		// should continue with the read
		if err := c.pull(ctx, branch); err != nil {
			if criticalErr := c.returnErr(err); criticalErr != nil {
				return criticalErr
			}
		}
	}
	// Do the read operation
	return operation()
}

func (c *Generic) getBranchLockInfo(branch string) *branchLock {
	c.branchLocksMu.Lock()
	defer c.branchLocksMu.Unlock()

	// Check if there exists a lock for that branch
	info, ok := c.branchLocks[branch]
	if ok {
		return info
	}
	// Write to the branchLocks map
	c.branchLocks[branch] = &branchLock{
		mu: &sync.RWMutex{},
	}
	return c.branchLocks[branch]
}

func (c *Generic) needsResync(branch string, d time.Duration) bool {
	lck := c.getBranchLockInfo(branch)
	// Lock while reading the last resync time
	lck.mu.RLock()
	defer lck.mu.RUnlock()
	// Resync if there has been no sync so far, or if the last resync was too long ago
	return lck.lastPull.IsZero() || time.Since(lck.lastPull) > d
}

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
func (c *Generic) StartResyncLoop(ctx context.Context, resyncCacheInterval time.Duration, resyncBranches ...string) {
	// Only start this loop if resyncCacheInterval > 0
	if resyncCacheInterval <= 0 {
		logrus.Warn("No need to start the resync loop; resyncCacheInterval <= 0")
		return
	}
	// If unset, only sync the default branch.
	if resyncBranches == nil {
		resyncBranches = []string{""}
	}

	// Start the resync goroutine
	go c.resyncLoop(ctx, resyncCacheInterval, resyncBranches)
}

func (c *Generic) resyncLoop(ctx context.Context, resyncCacheInterval time.Duration, resyncBranches []string) {
	logrus.Debug("Starting the resync loop...")

	wait.NonSlidingUntilWithContext(ctx, func(_ context.Context) {

		for _, branch := range resyncBranches {
			logrus.Tracef("resyncLoop: Will perform pull operation on branch: %q", branch)
			// Perform a fetch, pull & checkout of the new revision
			if err := c.pull(ctx, branch); err != nil {
				logrus.Errorf("resyncLoop: pull failed with error: %v", err)
				return
			}
		}
	}, resyncCacheInterval)
	logrus.Info("Exiting the resync loop...")
}

func (c *Generic) pull(ctx context.Context, branch string) error {
	// Need to get the branch-specific lock variable
	lck := c.getBranchLockInfo(branch)
	// Write-lock while this operation is in progress
	lck.mu.Lock()
	defer lck.mu.Unlock()

	// Create a new context that times out after the given duration
	pullCtx, cancel := context.WithTimeout(ctx, c.opts.PullTimeout)
	defer cancel()

	// Make a ctx for the given branch
	ctxForBranch := core.WithVersionRef(pullCtx, core.NewBranchRef(branch))
	if err := c.remote.Pull(ctxForBranch); err != nil {
		return err
	}

	// Register the timestamp into the lock
	lck.lastPull = time.Now()

	// All good
	return nil
}

func (c *Generic) PreTransactionHook(ctx context.Context, info transactional.TxInfo) error {
	// We count on ctx having the VersionRef registered for the head branch

	// Lock the branch for writing, if supported by the remote
	// If the lock fails, we DO NOT try to pull, but just exit (either with err or a nil error,
	// depending on the configured PACELC mode)
	// TODO: Can we rely on the timeout being exact enough here?
	// TODO: How to do this before the branch even exists...?
	if err := c.lock(ctx, info.Options.Timeout); err != nil {
		return c.returnErr(err)
	}

	// Always Pull the _base_ branch before a transaction, to be up-to-date
	// before creating the new head branch
	if err := c.pull(ctx, info.Base); err != nil {
		return c.returnErr(err)
	}

	// All good
	return nil
}

func (c *Generic) PreCommitHook(ctx context.Context, commit transactional.Commit, info transactional.TxInfo) error {
	return nil // nothing to do here
}

func (c *Generic) PostCommitHook(ctx context.Context, _ transactional.Commit, _ transactional.TxInfo) error {
	// Push the branch in the ctx
	if err := c.push(ctx); err != nil {
		return c.returnErr(err)
	}
	return nil
}

func (c *Generic) PostTransactionHook(ctx context.Context, info transactional.TxInfo) error {
	// Unlock the head branch, if supported
	if err := c.unlock(ctx); err != nil {
		return c.returnErr(err)
	}

	return nil
}

func (c *Generic) Remote() Remote {
	return c.remote
}

func (c *Generic) branchFromCtx(ctx context.Context) string {
	return core.GetVersionRef(ctx).Branch()
}

func (c *Generic) returnErr(err error) error {
	// If RemoteErrorStream isn't defined, just pass the error through
	if c.opts.RemoteErrorStream == nil {
		return err
	}
	// Non-blocking send to the channel, and no return error
	go func() {
		c.opts.RemoteErrorStream <- err
	}()
	return nil
}

func (c *Generic) lock(ctx context.Context, d time.Duration) error {
	lr, ok := c.remote.(LockableRemote)
	if !ok {
		return nil
	}

	// Need to get the branch-specific lock variable
	lck := c.getBranchLockInfo(c.branchFromCtx(ctx))
	// Write-lock while this operation is in progress
	lck.mu.Lock()
	defer lck.mu.Unlock()

	// Enforce a timeout
	lockCtx, cancel := context.WithTimeout(ctx, c.opts.LockTimeout)
	defer cancel()

	return lr.Lock(lockCtx, d)
}

func (c *Generic) unlock(ctx context.Context) error {
	lr, ok := c.remote.(LockableRemote)
	if !ok {
		return nil
	}

	// Need to get the branch-specific lock variable
	lck := c.getBranchLockInfo(c.branchFromCtx(ctx))
	// Write-lock while this operation is in progress
	lck.mu.Lock()
	defer lck.mu.Unlock()

	// Enforce a timeout
	unlockCtx, cancel := context.WithTimeout(ctx, c.opts.LockTimeout)
	defer cancel()

	return lr.Unlock(unlockCtx)
}

func (c *Generic) push(ctx context.Context) error {
	// Need to get the branch-specific lock variable
	lck := c.getBranchLockInfo(c.branchFromCtx(ctx))
	// Write-lock while this operation is in progress
	lck.mu.Lock()
	defer lck.mu.Unlock()

	// Create a new context that times out after the given duration
	pushCtx, cancel := context.WithTimeout(ctx, c.opts.PushTimeout)
	defer cancel()

	// Push the head branch using the remote
	// If the Push fails, don't execute any other later statements
	if err := c.remote.Push(pushCtx); err != nil {
		return err
	}
	return nil
}
