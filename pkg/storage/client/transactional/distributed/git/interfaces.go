package git

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Interface interface {
	Pull(ctx context.Context) error
	Fetch(ctx context.Context, revision string) error
	Push(ctx context.Context, branchName string) error
	CheckoutBranch(ctx context.Context, branchName string, force, create bool) error
	Clean(ctx context.Context) error
	FilesChanged(ctx context.Context, fromCommit, toCommit string) (sets.String, error)
	Commit(ctx context.Context, commit transactional.Commit) (string, error)
	IsWorktreeClean(ctx context.Context) (bool, error)
	ReadFileAtCommit(ctx context.Context, commit string, file string) ([]byte, error)
	CommitAt(ctx context.Context, branch string) (string, error)
}
