package transactional

import "context"

type TxInfo struct {
	Base    string
	Head    string
	Options TxOptions
}

type CommitHookChain interface {
	// The chain also itself implements CommitHook
	CommitHook
	// Register registers a new CommitHook to the chain
	Register(CommitHook)
}

// CommitHook executes directly before and after a commit is being made.
// If the transaction fails before a commit could happen, these will never
// be run.
type CommitHook interface {
	// PreCommitHook executes arbitrary logic for the given transaction info
	// and commit info; if an error is returned, the commit won't happen.
	PreCommitHook(ctx context.Context, commit Commit, info TxInfo) error
	// PostCommitHook executes arbitrary logic for the given transaction info
	// and commit info; if an error is returned, the commit will happen in the
	// case of a BranchTx on the head branch; but the transaction itself will
	// fail. In the case of a "normal" transaction; the commit will be made,
	// but later rolled back.
	PostCommitHook(ctx context.Context, commit Commit, info TxInfo) error
}

var _ CommitHookChain = &MultiCommitHook{}
var _ CommitHook = &MultiCommitHook{}

type MultiCommitHook struct {
	CommitHooks []CommitHook
}

func (m *MultiCommitHook) Register(h CommitHook) {
	m.CommitHooks = append(m.CommitHooks, h)
}

func (m *MultiCommitHook) PreCommitHook(ctx context.Context, commit Commit, info TxInfo) error {
	for _, ch := range m.CommitHooks {
		if ch == nil {
			continue
		}
		if err := ch.PreCommitHook(ctx, commit, info); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiCommitHook) PostCommitHook(ctx context.Context, commit Commit, info TxInfo) error {
	for _, ch := range m.CommitHooks {
		if ch == nil {
			continue
		}
		if err := ch.PostCommitHook(ctx, commit, info); err != nil {
			return err
		}
	}
	return nil
}

type TransactionHookChain interface {
	// The chain also itself implements TransactionHook
	TransactionHook
	// Register registers a new TransactionHook to the chain
	Register(TransactionHook)
}

// TransactionHook provides a way to extend transaction behavior. Regardless
// of the result of the transaction; these will always be run.
type TransactionHook interface {
	// PreTransactionHook executes before CreateBranch has been called for the
	// BranchManager in BranchTx mode; and in any case before any user-tx-specific
	// code starts executing.
	PreTransactionHook(ctx context.Context, info TxInfo) error
	// PostTransactionHook executes when a transaction is terminated, either due
	// to an Abort() or a successful Commit() or CreateTx().
	PostTransactionHook(ctx context.Context, info TxInfo) error
}

var _ TransactionHookChain = &MultiTransactionHook{}
var _ TransactionHook = &MultiTransactionHook{}

type MultiTransactionHook struct {
	TransactionHooks []TransactionHook
}

func (m *MultiTransactionHook) Register(h TransactionHook) {
	m.TransactionHooks = append(m.TransactionHooks, h)
}

func (m *MultiTransactionHook) PreTransactionHook(ctx context.Context, info TxInfo) error {
	for _, th := range m.TransactionHooks {
		if th == nil {
			continue
		}
		if err := th.PreTransactionHook(ctx, info); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiTransactionHook) PostTransactionHook(ctx context.Context, info TxInfo) error {
	for _, th := range m.TransactionHooks {
		if th == nil {
			continue
		}
		if err := th.PostTransactionHook(ctx, info); err != nil {
			return err
		}
	}
	return nil
}
