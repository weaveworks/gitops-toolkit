package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/fluxcd/go-git-providers/github"
	"github.com/fluxcd/go-git-providers/gitprovider"
	"github.com/labstack/echo"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/weaveworks/libgitops/cmd/common"
	"github.com/weaveworks/libgitops/cmd/common/logs"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/scheme"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/v1alpha1"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional/distributed"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional/distributed/git"
	githubpr "github.com/weaveworks/libgitops/pkg/storage/client/transactional/distributed/git/github"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/event"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	unstructuredfs "github.com/weaveworks/libgitops/pkg/storage/filesystem/unstructured"
	unstructuredevent "github.com/weaveworks/libgitops/pkg/storage/filesystem/unstructured/event"
	unstructuredtx "github.com/weaveworks/libgitops/pkg/storage/filesystem/unstructured/tx"
	"github.com/weaveworks/libgitops/pkg/storage/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	identityFlag    = pflag.String("identity-file", "", "Path to where the SSH private key is")
	authorNameFlag  = pflag.String("author-name", defaultAuthorName, "Author name for Git commits")
	authorEmailFlag = pflag.String("author-email", defaultAuthorEmail, "Author email for Git commits")
	gitURLFlag      = pflag.String("git-url", "", "HTTPS Git URL; where the Git repository is, e.g. https://github.com/luxas/ignite-gitops")
	prMilestoneFlag = pflag.String("pr-milestone", "", "What milestone to tag the PR with")
	prAssigneesFlag = pflag.StringSlice("pr-assignees", nil, "What user logins to assign for the created PR. The user must have pull access to the repo.")
	prLabelsFlag    = pflag.StringSlice("pr-labels", nil, "What labels to apply on the created PR. The labels must already exist. E.g. \"user/bot,actuator/libgitops,kind/status-update\"")
)

const (
	sshKnownHostsFile = "~/.ssh/known_hosts"

	defaultAuthorName  = "Weave libgitops"
	defaultAuthorEmail = "support@weave.works"
)

func main() {
	// Parse the version flag
	common.ParseVersionFlag()

	// Run the application
	if err := run(
		*identityFlag,
		*gitURLFlag,
		os.Getenv("GITHUB_TOKEN"),
		*authorNameFlag,
		*authorEmailFlag,
		*prMilestoneFlag,
		*prAssigneesFlag,
		*prLabelsFlag,
	); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func expandAndRead(filePath string) ([]byte, error) {
	expandedPath, err := homedir.Expand(filePath)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadFile(expandedPath)
}

func run(identityFile, gitURL, ghToken, authorName, authorEmail, prMilestone string,
	prAssignees, prLabels []string) error {
	// Validate parameters
	if len(identityFile) == 0 {
		return fmt.Errorf("--identity-file is required")
	}
	if len(gitURL) == 0 {
		return fmt.Errorf("--git-url is required")
	}
	if len(ghToken) == 0 {
		return fmt.Errorf("GITHUB_TOKEN is required")
	}
	if len(authorName) == 0 {
		return fmt.Errorf("--author-name is required")
	}
	if len(authorEmail) == 0 {
		return fmt.Errorf("--author-email is required")
	}

	// Set the log level
	logs.Logger.SetLevel(logrus.TraceLevel)

	// Read the identity and known_hosts files
	identityContent, err := expandAndRead(identityFile)
	if err != nil {
		return err
	}
	knownHostsContent, err := expandAndRead(sshKnownHostsFile)
	if err != nil {
		return err
	}

	// Parse the HTTPS clone URL
	repoRef, err := gitprovider.ParseOrgRepositoryURL(gitURL)
	if err != nil {
		return err
	}

	// Create a new GitHub client using the given token
	ghClient, err := github.NewClient(github.WithOAuth2Token(ghToken))
	if err != nil {
		return err
	}

	// Authenticate to the GitDirectory using Git SSH
	authMethod, err := git.NewSSHAuthMethod(identityContent, knownHostsContent)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())

	defer func() { cancel() }()

	// Construct the LocalClone implementation which backs the storage
	localClone, err := git.NewLocalClone(ctx, repoRef, git.LocalCloneOptions{
		Branch:     "master",
		AuthMethod: authMethod,
	})
	if err != nil {
		return err
	}

	ctx = core.WithVersionRef(ctx, core.NewBranchRef(localClone.MainBranch()))

	// Just use default encoders and decoders
	encoder := scheme.Serializer.Encoder()
	decoder := scheme.Serializer.Decoder()

	rawManifest, err := unstructuredevent.NewManifest(
		localClone.Dir(),
		filesystem.DefaultContentTyper,
		storage.StaticNamespacer{NamespacedIsDefaultPolicy: false}, // all objects root-spaced
		unstructuredfs.KubeObjectRecognizer{Decoder: decoder},
		filesystem.DefaultPathExcluders(),
	)
	if err != nil {
		return err
	}

	// Create the channel to receive events to, and register it with the EventStorage
	updates := make(event.ObjectEventStream, 4096)
	if err := rawManifest.WatchForObjectEvents(ctx, updates); err != nil {
		return err
	}

	defer func() { _ = rawManifest.Close() }()

	// Use the version information in the scheme to determine the storage version
	versioner := backend.SchemePreferredVersioner{Scheme: scheme.Scheme}

	b, err := backend.NewGeneric(rawManifest, encoder, decoder, kube.NewNamespaceEnforcer(), versioner, nil)
	if err != nil {
		return err
	}

	gitClient, err := client.NewGeneric(b)
	if err != nil {
		return err
	}

	txGeneralClient, err := transactional.NewGeneric(gitClient, localClone, nil)
	if err != nil {
		return err
	}

	// Note: This will add itself to the Commit/TxHook chains on the localClone.
	txClient, err := distributed.NewClient(txGeneralClient, localClone)
	if err != nil {
		return err
	}

	// Register a tx hook so that a new copy-on-write overlay is created when transactions are made
	versionRefHook := unstructuredtx.NewUnstructuredStorageTxHandler(gitClient)
	txClient.TransactionHookChain().Register(versionRefHook)

	// Create a new CommitHook for sending PRs
	prCommitHook, err := githubpr.NewGitHubPRCommitHandler(ghClient, localClone.RepositoryRef())
	if err != nil {
		return err
	}

	// Register the PR CommitHook with the distributed transaction Client
	// This needs to be done after the distributed.NewClient call, so
	// it has been able to handle pushing of the branch first.
	txClient.CommitHookChain().Register(prCommitHook)

	// Start the sync loop in the background
	txClient.StartResyncLoop(ctx, 15*time.Second)

	go func() {
		for upd := range updates {
			logrus.Infof("Got %s update for: %v %v", upd.Type, upd.ID.GroupKind(), upd.ID.ObjectKey())
		}
	}()

	e := common.NewEcho()

	e.GET("/git/", func(c echo.Context) error {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind("CarList"))

		/*if br := c.QueryParam("branch"); len(br) != 0 {
			ctx = core.WithVersionRef(ctx, core.NewBranchRef(br))
		}*/

		if err := txClient.List(ctx, list); err != nil {
			return err
		}
		var content bytes.Buffer
		if err := scheme.Serializer.Encoder().Encode(serializer.NewJSONFrameWriter(&content), list); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, content.Bytes())
	})

	e.PUT("/git/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		// Create an empty typed object, the data from the client will be written into it
		// at .Get-time below.
		car := v1alpha1.Car{}
		carKey := core.ObjectKey{Name: name}
		// Our head branch is the name of the Car, and it ends in a "-", which makes the
		// TxClient add a random sha suffix.
		headBranch := fmt.Sprintf("%s-update-", name)

		err := txClient.
			BranchTransaction(ctx, headBranch).      // Start a transaction of the base branch to the head
			Get(carKey, &car).                       // Load the latest data of the Car into &car.
			Custom(func(ctx context.Context) error { // Mutate (update) status of the Car
				car.Status.Distance = rand.Uint64()
				car.Status.Speed = rand.Float64() * 100
				return nil
			}).
			Update(&car).                         // Store the changed car in the Storage
			CreateTx(githubpr.GenericPullRequest{ // Create a commit for the tx; return the super-set PR commit
				Commit: transactional.GenericCommit{
					Author: transactional.GenericCommitAuthor{
						Name:  authorName,
						Email: authorEmail,
					},
					Message: transactional.GenericCommitMessage{
						Title:       "Update Car speed",
						Description: "We really need to sync this state!",
					},
				},
				Labels:    prLabels,
				Assignees: prAssignees,
				Milestone: prMilestone,
			}).Error()
		if err != nil {
			return err
		}

		return c.String(200, "OK!")
	})

	return common.StartEcho(e)
}
