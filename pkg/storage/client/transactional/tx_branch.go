package transactional

import (
	"context"
	"fmt"
)

type txBranchImpl struct {
	*txCommon

	merger BranchMerger
}

func (tx *txBranchImpl) CreateTx(c Commit) BranchTxResult {
	// Run the operations, and try to create the commit
	if err := tx.tryApplyAndCommitOperations(c); err != nil {
		// If we failed with the transaction, abort directly, and
		// return the error wrapped in a BranchTxResult
		abortErr := tx.Abort(err)
		return newErrTxResult(abortErr)
	}

	// We successfully completed all the tasks needed
	// Now, cleanup and unlock the branch
	cleanupErr := tx.cleanupFunc()

	// Allow the merger to merge, if supported
	return &txResultImpl{
		err:        cleanupErr,
		ctx:        tx.ctx,
		merger:     tx.merger,
		baseBranch: tx.info.Base,
		headBranch: tx.info.Head,
	}
}

func (tx *txBranchImpl) Custom(op CustomTxFunc) BranchTx {
	tx.ops = append(tx.ops, func() error {
		return op(tx.ctx)
	})
	return tx
}

func newErrTxResult(err error) *txResultImpl {
	return &txResultImpl{err: err}
}

type txResultImpl struct {
	err        error
	ctx        context.Context
	merger     BranchMerger
	baseBranch string
	headBranch string
}

func (r *txResultImpl) Error() error {
	return r.err
}

func (r *txResultImpl) MergeWithBase(c Commit) error {
	// If there is an internal error, return it
	if r.err != nil {
		return r.err
	}
	// Make sure we have a merger
	if r.merger == nil {
		return fmt.Errorf("TxResult: The BranchMerger is nil")
	}
	// Try to merge the branch
	return r.merger.MergeBranches(r.ctx, r.baseBranch, r.headBranch, c)
}
