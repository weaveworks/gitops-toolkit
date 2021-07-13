package transaction

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/gitdir"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/util"
	"github.com/weaveworks/libgitops/pkg/util/watcher"
)

var excludeDirs = []string{".git"}

func NewGitStorage(gitDir gitdir.GitDirectory, prProvider PullRequestProvider, ser serializer.Serializer) (TransactionStorage, error) {
	// Make sure the repo is cloned. If this func has already been called, it will be a no-op.
	if err := gitDir.StartCheckoutLoop(); err != nil {
		return nil, err
	}

	raw := storage.NewGenericMappedRawStorage(gitDir.Dir())
	s := storage.NewGenericStorage(raw, ser, []runtime.IdentifierFactory{runtime.Metav1NameIdentifier})

	gitStorage := &GitStorage{
		ReadStorage: s,
		s:           s,
		raw:         raw,
		gitDir:      gitDir,
		prProvider:  prProvider,
	}
	// Do a first sync now, and then start the background loop
	if err := gitStorage.sync(); err != nil {
		return nil, err
	}
	gitStorage.syncLoop()

	return gitStorage, nil
}

type GitStorage struct {
	storage.ReadStorage

	s          storage.Storage
	raw        storage.MappedRawStorage
	gitDir     gitdir.GitDirectory
	prProvider PullRequestProvider
}

func (s *GitStorage) syncLoop() {
	go func() {
		for {
			if commit, ok := <-s.gitDir.CommitChannel(); ok {
				logrus.Debugf("GitStorage: Got info about commit %q, syncing...", commit)
				if err := s.sync(); err != nil {
					logrus.Errorf("GitStorage: Got sync error: %v", err)
				}
			}
		}
	}()
}

func (s *GitStorage) sync() error {
	mappings, err := computeMappings(s.gitDir.Dir(), s.s)
	if err != nil {
		return err
	}
	logrus.Debugf("Rewriting the mappings to %v", mappings)
	s.raw.SetMappings(mappings)
	return nil
}

func (s *GitStorage) Transaction(ctx context.Context, streamName string, fn TransactionFunc) error {
	// Append random bytes to the end of the stream name if it ends with a dash
	if strings.HasSuffix(streamName, "-") {
		suffix, err := util.RandomSHA(4)
		if err != nil {
			return err
		}
		streamName += suffix
	}

	// Make sure we have the latest available state
	if err := s.gitDir.Pull(ctx); err != nil {
		return err
	}
	// Make sure no other Git ops can take place during the transaction, wait for other ongoing operations.
	s.gitDir.Suspend()
	defer s.gitDir.Resume()
	// Always switch back to the main branch afterwards.
	// TODO ordering of the defers, and return deferred error
	defer func() { _ = s.gitDir.CheckoutMainBranch() }()

	// Check out a new branch with the given name
	if err := s.gitDir.CheckoutNewBranch(streamName); err != nil {
		return err
	}
	// Invoke the transaction
	result, err := fn(ctx, s.s)
	if err != nil {
		return err
	}
	// Make sure the result is valid
	if err := result.Validate(); err != nil {
		return fmt.Errorf("transaction result is not valid: %w", err)
	}
	// Perform the commit
	if err := s.gitDir.Commit(ctx, result.GetAuthorName(), result.GetAuthorEmail(), result.GetMessage()); err != nil {
		return err
	}
	// Return if no PR should be made
	prResult, ok := result.(PullRequestResult)
	if !ok {
		return nil
	}
	// If a PR was asked for, and no provider was given, error out
	if s.prProvider == nil {
		return ErrNoPullRequestProvider
	}
	// Create the PR using the provider.
	return s.prProvider.CreatePullRequest(ctx, &GenericPullRequestSpec{
		PullRequestResult: prResult,
		MainBranch:        s.gitDir.MainBranch(),
		MergeBranch:       streamName,
		RepositoryRef:     s.gitDir.RepositoryRef(),
	})
}

func computeMappings(dir string, s storage.Storage) (map[storage.ObjectKey]string, error) {
	validExts := make([]string, 0, len(storage.ContentTypes))
	for ext := range storage.ContentTypes {
		validExts = append(validExts, ext)
	}

	files, err := watcher.WalkDirectoryForFiles(dir, validExts, excludeDirs)
	if err != nil {
		return nil, err
	}

	// TODO: Compute the difference between the earlier state, and implement EventStorage so the user
	// can automatically subscribe to changes of objects between versions.
	m := map[storage.ObjectKey]string{}
	for _, file := range files {
		partObjs, err := storage.DecodePartialObjects(frame.FromFile(file), s.Serializer().Scheme(), false, nil)
		if err != nil {
			logrus.Errorf("couldn't decode %q into a partial object: %v", file, err)
			continue
		}
		key, err := s.ObjectKeyFor(partObjs[0])
		if err != nil {
			logrus.Errorf("couldn't get objectkey for partial object: %v", err)
			continue
		}
		logrus.Debugf("Adding mapping between %s and %q", key, file)
		m[key] = file
	}
	return m, nil
}
