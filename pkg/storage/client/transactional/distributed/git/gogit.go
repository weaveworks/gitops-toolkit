package git

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fluxcd/go-git-providers/gitprovider"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"k8s.io/apimachinery/pkg/util/sets"
)

func NewGoGit(ctx context.Context, repoRef gitprovider.RepositoryRef, dir string, opts *Options) (Interface, error) {
	gg := &goGit{
		repoRef: repoRef,
		dir:     dir,
		lock:    &sync.Mutex{},
		opts:    opts,
	}
	// Clone to populate repo & wt
	if err := gg.clone(ctx); err != nil {
		return nil, err
	}
	return gg, nil
}

type goGit struct {
	repoRef gitprovider.RepositoryRef
	dir     string
	lock    *sync.Mutex
	opts    *Options

	// go-git objects. wt is the worktree of the repo, persistent during the lifetime of repo.
	repo *git.Repository
	wt   *git.Worktree
}

func (g *goGit) clone(ctx context.Context) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	g.lock.Lock()
	defer g.lock.Unlock()

	transportType := gitprovider.TransportTypeHTTPS // default
	if g.opts.AuthMethod != nil {
		// TODO: parse the URL instead
		transportType = g.opts.AuthMethod.TransportType()
	}
	cloneURL := g.repoRef.GetCloneURL(transportType)

	cloneOpts := &git.CloneOptions{
		URL:          cloneURL,
		Auth:         g.opts.AuthMethod,
		SingleBranch: true,
		NoCheckout:   false,
		//Depth:             1, // ref: https://github.com/go-git/go-git/issues/207
		RecurseSubmodules: 0,
		Progress:          nil,
		Tags:              git.NoTags,
	}
	if g.opts.MainBranch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(g.opts.MainBranch)
	}

	log.Infof("Starting to clone the repository %s", g.repoRef)
	// Do a clone operation to the temporary directory
	var err error
	g.repo, err = git.PlainCloneContext(ctx, g.dir, false, cloneOpts)
	// Handle errors
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("git clone operation timed out: %w", err)
	} else if errors.Is(err, context.Canceled) {
		return fmt.Errorf("git clone was cancelled: %w", err)
	} else if err != nil {
		return fmt.Errorf("git clone error: %v", err)
	}

	// Populate the worktree pointer
	g.wt, err = g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("git get worktree error: %v", err)
	}

	// Get the latest HEAD commit and report it to the user
	ref, err := g.repo.Head()
	if err != nil {
		return err
	}

	log.Infof("Repo cloned; HEAD commit is %s", ref.Hash())
	return nil
}

func (g *goGit) Pull(ctx context.Context) error {
	// Perform the git pull operation. The context carries a timeout
	log.Trace("Starting pull operation")
	err := g.wt.PullContext(ctx, &git.PullOptions{
		Auth:         g.opts.AuthMethod,
		SingleBranch: true,
	})

	// Handle errors
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		// all good, nothing more to do
		log.Trace("Pull already up-to-date")
		return nil
	} else if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("git pull operation timed out: %w", err)
	} else if errors.Is(err, context.Canceled) {
		return fmt.Errorf("git pull was cancelled: %w", err)
	} else if err != nil {
		return fmt.Errorf("git pull error: %v", err)
	}

	log.Trace("Pulled successfully")
	return nil
}

func (g *goGit) Push(ctx context.Context, branchName string) error {
	opts := &git.PushOptions{
		Auth: g.opts.AuthMethod,
	}
	// Only push the branch in question, if set
	if branchName != "" {
		opts.RefSpecs = sameRevisionRefSpecs(branchName)
	}

	err := g.repo.PushContext(ctx, opts)
	// Handle errors
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		// TODO: Is it good if there's nothing more to do; or a failure if there's nothing to push?
		log.Trace("Push already up-to-date")
		return nil
	} else if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("git push operation timed out: %w", err)
	} else if errors.Is(err, context.Canceled) {
		return fmt.Errorf("git push was cancelled: %w", err)
	} else if err != nil {
		return fmt.Errorf("git push error: %v", err)
	}

	log.Trace("Pushed successfully")
	return nil
}

func (g *goGit) Fetch(ctx context.Context, revision string) error {
	// Perform the git pull operation. The context carries a timeout
	log.Trace("Starting pull operation")
	err := g.repo.FetchContext(ctx, &git.FetchOptions{
		Auth: g.opts.AuthMethod,
		// Fetch exactly this ref, and not others
		RefSpecs: sameRevisionRefSpecs(revision),
	})

	// Handle errors
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		// all good, nothing more to do
		log.Trace("Fetch already up-to-date")
		return nil
	} else if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("git fetch operation timed out: %w", err)
	} else if errors.Is(err, context.Canceled) {
		return fmt.Errorf("git fetch was cancelled: %w", err)
	} else if err != nil {
		return fmt.Errorf("git fetch error: %v", err)
	}

	log.Trace("Fetched successfully")
	return nil
}

func (g *goGit) CheckoutBranch(ctx context.Context, branch string, force, create bool) error {
	return g.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branch),
		Force:  true,
		Create: create,
	})
}

func (g *goGit) Clean(_ context.Context) error {
	// This is essentially a "git clean -f -d ."
	return g.wt.Clean(&git.CleanOptions{
		Dir: true,
	})
}

func (g *goGit) FilesChanged(ctx context.Context, fromCommit, toCommit string) (sets.String, error) {
	from, err := g.repo.CommitObject(plumbing.NewHash(fromCommit))
	if err != nil {
		return nil, err
	}
	//s, e := cA.Stats()
	//s[0].
	//ci, err := g.repo.CommitObjects()
	ci, err := g.repo.Log(&git.LogOptions{
		From:  plumbing.NewHash(toCommit),
		Order: git.LogOrderCommitterTime,
		Since: &from.Author.When,
	})
	if err != nil {
		return nil, err
	}
	files := sets.NewString()
	err = ci.ForEach(func(c *object.Commit) error {
		filesChanged, err := c.StatsContext(ctx)
		if err != nil {
			return err
		}
		for _, fileChanged := range filesChanged {
			files.Insert(fileChanged.Name)
		}
		return nil
	})
	return files, err
}

func (g *goGit) Commit(_ context.Context, commit transactional.Commit) (string, error) {
	hash, err := g.wt.Commit(commit.GetMessage().String(), &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  commit.GetAuthor().GetName(),
			Email: commit.GetAuthor().GetEmail(),
			When:  time.Now(),
		},
	})
	return hash.String(), err
}
func (g *goGit) IsWorktreeClean(_ context.Context) (bool, error) {
	s, err := g.wt.Status()
	if err != nil {
		return false, fmt.Errorf("git status failed: %v", err)
	}
	return s.IsClean(), nil
}

func (g *goGit) ReadFileAtCommit(_ context.Context, commit string, file string) ([]byte, error) {
	c, err := g.repo.CommitObject(plumbing.NewHash(commit))
	if err != nil {
		return nil, err
	}
	f, err := c.File(file)
	if err != nil {
		return nil, err
	}
	content, err := f.Contents()
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}
func (g *goGit) CommitAt(_ context.Context, branch string) (rev string, err error) {
	var reference *plumbing.Reference
	if branch != "" { // Point at HEAD
		reference, err = g.repo.Head()
	} else {
		reference, err = g.repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	}
	if err != nil {
		return
	}
	return reference.Hash().String(), nil
}

// assume either the revision is a hash or a branch
func sameRevisionRefSpecs(revision string) []config.RefSpec {
	if plumbing.IsHash(revision) {
		revision = fmt.Sprintf("%s:%s", revision, revision)
	} else {
		revision = fmt.Sprintf("refs/heads/%s:refs/heads/%s", revision, revision)
	}
	return []config.RefSpec{config.RefSpec(revision)}
}
