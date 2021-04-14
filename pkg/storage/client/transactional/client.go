package transactional

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	utilerrs "k8s.io/apimachinery/pkg/util/errors"
)

var _ Client = &Generic{}

func NewGeneric(c client.Client, manager BranchManager, merger BranchMerger) (Client, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: c is required", core.ErrInvalidParameter)
	}
	if manager == nil {
		return nil, fmt.Errorf("%w: manager is required", core.ErrInvalidParameter)
	}
	return &Generic{
		c:           c,
		txs:         make(map[string]*txLock),
		txsMu:       &sync.Mutex{},
		txHooks:     &MultiTransactionHook{},
		commitHooks: &MultiCommitHook{},
		manager:     manager,
		merger:      merger,
	}, nil
}

type Generic struct {
	c client.Client

	// txs maps branches to their tx locks
	txs map[string]*txLock
	// txsMu guards reads and writes of txs
	txsMu *sync.Mutex

	// Hooks
	txHooks     TransactionHookChain
	commitHooks CommitHookChain

	// +optional
	merger BranchMerger
	// +required
	manager BranchManager
}

type txLock struct {
	// mu is locked for writing while the transaction is executing, and locked
	// for reading, while a read operation is active.
	mu *sync.RWMutex
	// mode specifies what transaction mode is used; Atomic or AllowReading.
	mode TxMode
	// active == 1 means "transaction active, mu is locked for writing"
	// active == 0 means "transaction has stopped, mu has been unlocked"
	active uint32
}

func (c *Generic) Get(ctx context.Context, key core.ObjectKey, obj client.Object) error {
	return c.lockAndReadBranch(ctx, func() error {
		return c.c.Get(ctx, key, obj)
	})
}

func (c *Generic) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.lockAndReadBranch(ctx, func() error {
		return c.c.List(ctx, list, opts...)
	})
}

func (c *Generic) getBranchLockInfo(branch string) *txLock {
	// c.txsMu guards reads and writes to the c.txs map
	c.txsMu.Lock()
	defer c.txsMu.Unlock()

	// Check if information about a transaction on this branch exists.
	txState, ok := c.txs[branch]
	if ok {
		return txState
	}
	// if not, grow the txs map by one and return it
	c.txs[branch] = &txLock{
		mu: &sync.RWMutex{},
	}
	return c.txs[branch]
}

func (c *Generic) lockAndReadBranch(ctx context.Context, callback func() error) error {
	// Aquire the branch-specific lock
	branch := core.GetVersionRef(ctx).Branch()
	txState := c.getBranchLockInfo(branch)

	// In the atomic mode, we lock the txLock during the read,
	// so no new transactions can be started while the read
	// operation goes on. In non-atomic modes, reads aren't locked,
	// instead it is assumed that downstream implementations just
	// read the latest commit on the given branch.
	if txState.mode == TxModeAtomic {
		txState.mu.RLock()
	}
	err := callback()
	if txState.mode == TxModeAtomic {
		txState.mu.RUnlock()
	}
	return err
}

func (c *Generic) initTx(ctx context.Context, info TxInfo) (context.Context, txFunc) {
	// Aquire the branch-specific lock
	txState := c.getBranchLockInfo(info.Head)

	// Wait for all reads to complete (in the case of the atomic more),
	// and then lock for writing. For non-atomic mode this uses the mutex
	// as it is modifying txState, and two transactions must not run at
	// the same time for the same branch.
	//
	// Always lock mu when a transaction is running on this branch,
	// regardless of mode. If atomic mode is enabled, this also waits
	// on any reads happening at this moment. For all modes, this ensures
	// transactions happen in order.
	txState.mu.Lock()
	txState.active = 1               // set tx state to "active"
	txState.mode = info.Options.Mode // declare what transaction mode is used

	// Create a child context with a timeout
	dlCtx, cleanupTimeout := context.WithTimeout(ctx, info.Options.Timeout)

	// This function cleans up the transaction, and unlocks the tx muted
	cleanupFunc := func() error {
		// Cleanup after the transaction
		if err := c.cleanupAfterTx(ctx, &info); err != nil {
			return fmt.Errorf("Failed to cleanup branch %s after tx: %v", info.Head, err)
		}
		// Unlock the mutex so new transactions can take place on this branch
		txState.mu.Unlock()
		return nil
	}

	// Start waiting for the cancellation of the deadline context.
	go func() {
		// Wait for the context to either timeout or be cancelled
		<-dlCtx.Done()
		// This guard makes sure the cleanup function runs exactly
		// once, regardless of transaction end cause.
		if atomic.CompareAndSwapUint32(&txState.active, 1, 0) {
			if err := cleanupFunc(); err != nil {
				logrus.Errorf("Failed to cleanup after tx timeout: %v", err)
			}
		}
	}()

	abortFunc := func() error {
		// The transaction ended; the caller is either Abort() or
		// at the end of a successful transaction. The cause of
		// Abort() happening can also be a context cancellation.
		// If the parent context was cancelled or timed out; this
		// function and the above function race to set active => 0
		// Regardless, due to the atomic nature of the operation,
		// cleanupFunc() will only be run twice.
		if atomic.CompareAndSwapUint32(&txState.active, 1, 0) {
			// We can now stop the timeout timer
			cleanupTimeout()
			// Clean up the transaction
			return cleanupFunc()
		}
		return nil
	}

	return dlCtx, abortFunc
}

func (c *Generic) cleanupAfterTx(ctx context.Context, info *TxInfo) error {
	// Always both clean the branch, and run post-tx tasks
	return utilerrs.NewAggregate([]error{
		c.manager.ResetToCleanBranch(ctx, info.Base),
		// TODO: should this be in its own goroutine to switch back to main
		// ASAP?
		c.TransactionHookChain().PostTransactionHook(ctx, *info),
	})
}

func (c *Generic) BackendReader() backend.Reader {
	return c.c.BackendReader()
}

func (c *Generic) BranchMerger() BranchMerger {
	return c.merger
}

func (c *Generic) BranchManager() BranchManager {
	return c.manager
}

func (c *Generic) TransactionHookChain() TransactionHookChain {
	return c.txHooks
}

func (c *Generic) CommitHookChain() CommitHookChain {
	return c.commitHooks
}

func (c *Generic) Transaction(ctx context.Context, opts ...TxOption) Tx {
	tx, err := c.transaction(ctx, opts...)
	if err != nil {
		panic(err)
	}
	return tx
}

func (c *Generic) BranchTransaction(ctx context.Context, headBranch string, opts ...TxOption) BranchTx {
	tx, err := c.branchTransaction(ctx, headBranch, opts...)
	if err != nil {
		panic(err)
	}
	return tx
}

func (c *Generic) transaction(ctx context.Context, opts ...TxOption) (Tx, error) {

	// Parse options
	o := defaultTxOptions().ApplyOptions(opts)

	branch := core.GetVersionRef(ctx).Branch()
	info := TxInfo{
		Base:    branch,
		Head:    branch,
		Options: *o,
	}
	// Initialize the transaction
	ctxWithDeadline, cleanupFunc := c.initTx(ctx, info)

	// Run pre-tx checks
	err := c.TransactionHookChain().PreTransactionHook(ctxWithDeadline, info)

	return &txImpl{
		&txCommon{
			err:         err,
			c:           c.c,
			manager:     c.manager,
			commitHook:  c.CommitHookChain(),
			ctx:         ctxWithDeadline,
			info:        info,
			cleanupFunc: cleanupFunc,
		},
	}, nil
}

func (c *Generic) branchTransaction(ctx context.Context, headBranch string, opts ...TxOption) (BranchTx, error) {
	baseBranch := core.GetVersionRef(ctx).Branch()

	// Append random bytes to the end of the head branch if it ends with a dash
	if strings.HasSuffix(headBranch, "-") {
		suffix, err := randomSHA(4)
		if err != nil {
			return nil, err
		}
		headBranch += suffix
	}

	// Validate that the base and head branches are distinct
	if baseBranch == headBranch {
		return nil, fmt.Errorf("head and target branches must not be the same")
	}

	logrus.Debugf("Base branch: %q. Head branch: %q.", baseBranch, headBranch)

	// Parse options
	o := defaultTxOptions().ApplyOptions(opts)

	info := TxInfo{
		Base:    baseBranch,
		Head:    headBranch,
		Options: *o,
	}

	// Register the head branch with the context
	ctxWithHeadBranch := core.WithVersionRef(ctx, core.NewBranchRef(headBranch))
	// Initialize the transaction
	ctxWithDeadline, cleanupFunc := c.initTx(ctxWithHeadBranch, info)

	// Run pre-tx checks and create the new branch
	err := utilerrs.NewAggregate([]error{
		c.TransactionHookChain().PreTransactionHook(ctxWithDeadline, info),
		c.manager.CreateBranch(ctxWithDeadline, headBranch),
	})

	return &txBranchImpl{
		txCommon: &txCommon{
			err:         err,
			c:           c.c,
			manager:     c.manager,
			commitHook:  c.CommitHookChain(),
			ctx:         ctxWithDeadline,
			info:        info,
			cleanupFunc: cleanupFunc,
		},
		merger: c.merger,
	}, nil
}
