package transactional

type txImpl struct {
	*txCommon
}

func (tx *txImpl) Commit(c Commit) error {
	// Run the operations, and try to create the commit
	if err := tx.tryApplyAndCommitOperations(c); err != nil {
		// If we failed with the transaction, abort directly
		return tx.Abort(err)
	}

	// We successfully completed all the tasks needed
	// Now, cleanup and unlock the branch
	return tx.cleanupFunc()
}

func (tx *txImpl) Custom(op CustomTxFunc) Tx {
	tx.ops = append(tx.ops, func() error {
		return op(tx.ctx)
	})
	return tx
}
