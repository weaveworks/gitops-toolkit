package transactional

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

type Client interface {
	client.Reader

	BranchManager() BranchManager
	// BranchMerger is optional.
	BranchMerger() BranchMerger

	// CommitHookChain is a chain of hooks that are run before and after a commit is made.
	CommitHookChain() CommitHookChain
	// TransactionHookChain is a chain of hooks that are run before and after a transaction.
	TransactionHookChain() TransactionHookChain

	// Transaction creates a new transaction on the branch stored in the context, so that
	// no other writes to that branch can take place meanwhile.
	Transaction(ctx context.Context, opts ...TxOption) Tx
	// BranchTransaction creates a new "head" branch with the given {branchName} name, based
	// on the "base" branch in the context. The "base" branch is not locked for writing while
	// the transaction is running, but the head branch is.
	BranchTransaction(ctx context.Context, branchName string, opts ...TxOption) BranchTx
}

type BranchManager interface {
	// CreateBranch creates a new branch with the given target branch name. It forks out
	// of the branch specified in the context.
	CreateBranch(ctx context.Context, branch string) error
	// ResetToCleanBranch switches back to the given branch; but first discards all non-committed
	// changes.
	ResetToCleanBranch(ctx context.Context, branch string) error
	// Commit creates a new commit for the branch stored in the context.
	Commit(ctx context.Context, commit Commit) error
}

type BranchMerger interface {
	MergeBranches(ctx context.Context, base, head string, commit Commit) error
}

type CustomTxFunc func(ctx context.Context) error

type Tx interface {
	Commit(Commit) error
	Abort(err error) error

	Client() client.Client

	Custom(CustomTxFunc) Tx

	Get(key core.ObjectKey, obj client.Object) Tx
	List(list client.ObjectList, opts ...client.ListOption) Tx

	Create(obj client.Object, opts ...client.CreateOption) Tx
	Update(obj client.Object, opts ...client.UpdateOption) Tx
	Patch(obj client.Object, patch client.Patch, opts ...client.PatchOption) Tx
	Delete(obj client.Object, opts ...client.DeleteOption) Tx
	DeleteAllOf(obj client.Object, opts ...client.DeleteAllOfOption) Tx

	UpdateStatus(obj client.Object, opts ...client.UpdateOption) Tx
	PatchStatus(obj client.Object, patch client.Patch, opts ...client.PatchOption) Tx
}

type BranchTx interface {
	CreateTx(Commit) BranchTxResult
	Abort(err error) error

	Client() client.Client

	Custom(CustomTxFunc) BranchTx

	Get(key core.ObjectKey, obj client.Object) BranchTx
	List(list client.ObjectList, opts ...client.ListOption) BranchTx

	Create(obj client.Object, opts ...client.CreateOption) BranchTx
	Update(obj client.Object, opts ...client.UpdateOption) BranchTx
	Patch(obj client.Object, patch client.Patch, opts ...client.PatchOption) BranchTx
	Delete(obj client.Object, opts ...client.DeleteOption) BranchTx
	DeleteAllOf(obj client.Object, opts ...client.DeleteAllOfOption) BranchTx

	UpdateStatus(obj client.Object, opts ...client.UpdateOption) BranchTx
	PatchStatus(obj client.Object, patch client.Patch, opts ...client.PatchOption) BranchTx
}

type BranchTxResult interface {
	Error() error
	MergeWithBase(Commit) error
}
