package git

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"github.com/fluxcd/go-git-providers/gitprovider"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional/distributed"
)

var (
	// ErrNotStarted happens if you try to operate on the LocalClone before you have started
	// it with StartCheckoutLoop.
	ErrNotStarted = errors.New("the LocalClone hasn't been started (and hence, cloned) yet")
	// ErrCannotWriteToReadOnly happens if you try to do a write operation for a non-authenticated Git repo.
	ErrCannotWriteToReadOnly = errors.New("the LocalClone is read-only, cannot write")
	// ErrWorktreeClean happens if there are no modified files in the worktree when trying to create a commit.
	ErrWorktreeClean = errors.New("there are no modified files, cannot create new commit")
	// ErrWorktreeNotClean happens if there are modified files in the worktree when trying to create a new branch.
	ErrWorktreeNotClean = errors.New("there are uncommitted changes, cannot create new branch")
)

// LocalClone is an implementation of both a Remote, and a BranchManager, for Git.
var _ transactional.BranchManager = &LocalClone{}
var _ distributed.Remote = &LocalClone{}

// Create a new Remote and BranchManager implementation using Git. The repo is cloned immediately
// in the constructor, you can use ctx to enforce a timeout for the clone.
func NewLocalClone(ctx context.Context, repoRef gitprovider.RepositoryRef, opts ...Option) (*LocalClone, error) {
	log.Info("Initializing the Git repo...")

	o := defaultOpts().ApplyOptions(opts)

	// Create a temporary directory for the clone
	tmpDir, err := ioutil.TempDir("", "libgitops")
	if err != nil {
		return nil, err
	}
	log.Debugf("Created temporary directory for the git clone at %q", tmpDir)

	d := &LocalClone{
		repoRef:  repoRef,
		opts:     o,
		cloneDir: tmpDir,
		lock:     &sync.Mutex{},
	}

	log.Trace("URL endpoint parsed and authentication method chosen")

	if d.canWrite() {
		log.Infof("Running in read-write mode, will commit back current status to the repo")
	} else {
		log.Infof("Running in read-only mode, won't write status back to the repo")
	}

	d.impl, err = NewGoGit(ctx, repoRef, tmpDir, o)
	if err != nil {
		return nil, err
	}

	return d, nil
}

// LocalClone is an implementation of both a Remote, and a BranchManager, for Git.
// TODO: Make so that the LocalClone does NOT interfere with any reads or writes by the Client using some shared
// mutex.
type LocalClone struct {
	// user-specified options
	repoRef gitprovider.RepositoryRef
	opts    *Options

	// the temporary directory used for the clone
	cloneDir string

	// the lock for git operations (so no ops are done simultaneously)
	lock *sync.Mutex

	impl Interface

	// TODO: Keep track of current worktree branch
}

func (d *LocalClone) Dir() string {
	return d.cloneDir
}

func (d *LocalClone) MainBranch() string {
	return d.opts.MainBranch
}

func (d *LocalClone) RepositoryRef() gitprovider.RepositoryRef {
	return d.repoRef
}

func (d *LocalClone) canWrite() bool {
	return d.opts.AuthMethod != nil
}

// verifyRead makes sure it's ok to start a read-something-from-git process
func (d *LocalClone) verifyRead() error {
	// Safeguard against not starting yet
	/*if d.wt == nil {
		return fmt.Errorf("cannot pull: %w", ErrNotStarted)
	}*/
	return nil
}

// verifyWrite makes sure it's ok to start a write-something-to-git process
func (d *LocalClone) verifyWrite() error {
	// We need all read privileges first
	if err := d.verifyRead(); err != nil {
		return err
	}
	// Make sure we don't write to a possibly read-only repo
	if !d.canWrite() {
		return ErrCannotWriteToReadOnly
	}
	return nil
}

func (d *LocalClone) Pull(ctx context.Context) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	// TODO: This should support doing Fetch() only maybe
	// TODO: Remove the requirement to actually be on the branch
	// that is being pulled.

	// Make sure it's okay to read
	if err := d.verifyRead(); err != nil {
		return err
	}

	if err := d.impl.Pull(ctx); err != nil {
		return err
	}

	ref, err := d.impl.CommitAt(ctx, "") // HEAD
	if err != nil {
		return err
	}

	log.Infof("New commit observed %s", ref)
	return nil
}

func (d *LocalClone) Push(ctx context.Context) error {
	// Perform the git push operation. The context carries a timeout
	log.Debug("Starting push operation")
	return d.impl.Push(ctx, "") // TODO: only push the current branch
}

func (d *LocalClone) CreateBranch(ctx context.Context, branch string) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	// TODO: Should the caller do a force-reset using ResetToCleanBranch before creating the branch?

	// Make sure it's okay to write
	if err := d.verifyWrite(); err != nil {
		return err
	}

	// Sanity-check that the worktree is clean before switching branches
	if clean, err := d.impl.IsWorktreeClean(ctx); err != nil {
		return err
	} else if !clean {
		return ErrWorktreeNotClean
	}

	// Create and switch to the new branch
	return d.impl.CheckoutBranch(ctx, branch, false, true)
}

func (d *LocalClone) ResetToCleanBranch(ctx context.Context, branch string) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	// Make sure it's okay to write
	if err := d.verifyWrite(); err != nil {
		return err
	}

	// Best-effort clean, don't check the error
	_ = d.impl.Clean(ctx)
	// Force-checkout the main branch
	// TODO: If a transaction (non-branched) was able to commit, and failed after that
	// we need to roll back that commit.
	return d.impl.CheckoutBranch(ctx, branch, true, false)
	// TODO: Do a pull here too?
}

// Commit creates a commit of all changes in the current worktree with the given parameters.
// It also automatically pushes the branch after the commit.
// ErrNotStarted is returned if the repo hasn't been cloned yet.
// ErrCannotWriteToReadOnly is returned if opts.AuthMethod wasn't provided.
func (d *LocalClone) Commit(ctx context.Context, commit transactional.Commit) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	// Make sure it's okay to write
	if err := d.verifyWrite(); err != nil {
		return err
	}

	// Don't commit anything if already clean
	if clean, err := d.impl.IsWorktreeClean(ctx); err != nil {
		return err
	} else if clean {
		return ErrWorktreeClean
	}

	// Do a commit
	log.Debug("Committing all local changes")
	hash, err := d.impl.Commit(ctx, commit)
	if err != nil {
		return fmt.Errorf("git commit error: %v", err)
	}

	// Notify upstream that we now have a new commit, and allow writing again
	log.Infof("A new commit has been created: %q", hash)
	return nil
}

// Cleanup cancels running goroutines and operations, and removes the temporary clone directory
func (d *LocalClone) Cleanup() error {
	// Remove the temporary directory
	if err := os.RemoveAll(d.Dir()); err != nil {
		log.Errorf("Failed to clean up temp git directory: %v", err)
		return err
	}
	return nil
}
