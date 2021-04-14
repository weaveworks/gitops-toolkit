package github

import (
	"context"
	"errors"
	"fmt"

	"github.com/fluxcd/go-git-providers/github"
	"github.com/fluxcd/go-git-providers/gitprovider"
	"github.com/fluxcd/go-git-providers/validation"
	gogithub "github.com/google/go-github/v32/github"
	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
)

// PullRequest can be returned from a TransactionFunc instead of a CommitResult, if
// a PullRequest is desired to be created by the PullRequestProvider.
type PullRequest interface {
	// PullRequestResult is a superset of CommitResult
	transactional.Commit

	// GetLabels specifies what labels should be applied on the PR.
	// +optional
	GetLabels() []string
	// GetAssignees specifies what user login names should be assigned to this PR.
	// Note: Only users with "pull" access or more can be assigned.
	// +optional
	GetAssignees() []string
	// GetMilestone specifies what milestone this should be attached to.
	// +optional
	GetMilestone() string
}

// GenericPullRequest implements PullRequest.
var _ PullRequest = GenericPullRequest{}

// GenericPullRequest implements PullRequest.
type GenericPullRequest struct {
	// GenericPullRequest is a superset of a Commit.
	transactional.Commit

	// Labels specifies what labels should be applied on the PR.
	// +optional
	Labels []string
	// Assignees specifies what user login names should be assigned to this PR.
	// Note: Only users with "pull" access or more can be assigned.
	// +optional
	Assignees []string
	// Milestone specifies what milestone this should be attached to.
	// +optional
	Milestone string
}

func (r GenericPullRequest) GetLabels() []string    { return r.Labels }
func (r GenericPullRequest) GetAssignees() []string { return r.Assignees }
func (r GenericPullRequest) GetMilestone() string   { return r.Milestone }

func (r GenericPullRequest) Validate() error {
	v := validation.New("GenericPullRequest")
	// Just validate the "inner" object
	v.Append(r.Commit.Validate(), r.Commit, "Commit")
	return v.Error()
}

// TODO: This package should really only depend on go-git-providers' abstraction interface

var ErrProviderNotSupported = errors.New("only the Github go-git-providers provider is supported at the moment")

// NewGitHubPRCommitHandler returns a new transactional.CommitHandler from a gitprovider.Client.
func NewGitHubPRCommitHandler(c gitprovider.Client, repoRef gitprovider.RepositoryRef) (transactional.CommitHook, error) {
	// Make sure a Github client was passed
	if c.ProviderID() != github.ProviderID {
		return nil, ErrProviderNotSupported
	}
	return &prCreator{c, repoRef}, nil
}

type prCreator struct {
	c       gitprovider.Client
	repoRef gitprovider.RepositoryRef
}

func (c *prCreator) PreCommitHook(ctx context.Context, commit transactional.Commit, info transactional.TxInfo) error {
	return nil
}

func (c *prCreator) PostCommitHook(ctx context.Context, commit transactional.Commit, info transactional.TxInfo) error {
	// First, validate the input
	if err := commit.Validate(); err != nil {
		return fmt.Errorf("given transactional.Commit wasn't valid")
	}

	prCommit, ok := commit.(PullRequest)
	if !ok {
		return nil
	}

	// Use the "raw" go-github client to do this
	ghClient := c.c.Raw().(*gogithub.Client)

	// Helper variables
	owner := c.repoRef.GetIdentity()
	repo := c.repoRef.GetRepository()
	var body *string
	if commit.GetMessage().GetDescription() != "" {
		body = gogithub.String(commit.GetMessage().GetDescription())
	}

	// Create the Pull Request
	prPayload := &gogithub.NewPullRequest{
		Head:  gogithub.String(info.Head),
		Base:  gogithub.String(info.Base),
		Title: gogithub.String(commit.GetMessage().GetTitle()),
		Body:  body,
	}
	logrus.Infof("GitHub PR payload: %+v", prPayload)
	pr, _, err := ghClient.PullRequests.Create(ctx, owner, repo, prPayload)
	if err != nil {
		return err
	}

	// If spec.GetMilestone() is set, fetch the ID of the milestone
	// Only set milestoneID to non-nil if specified
	var milestoneID *int
	if len(prCommit.GetMilestone()) != 0 {
		milestoneID, err = getMilestoneID(ctx, ghClient, owner, repo, prCommit.GetMilestone())
		if err != nil {
			return err
		}
	}

	// Only set assignees to non-nil if specified
	var assignees *[]string
	if a := prCommit.GetAssignees(); len(a) != 0 {
		assignees = &a
	}

	// Only set labels to non-nil if specified
	var labels *[]string
	if l := prCommit.GetLabels(); len(l) != 0 {
		labels = &l
	}

	// Only PATCH the PR if any of the fields were set
	if milestoneID != nil || assignees != nil || labels != nil {
		_, _, err := ghClient.Issues.Edit(ctx, owner, repo, pr.GetNumber(), &gogithub.IssueRequest{
			Milestone: milestoneID,
			Assignees: assignees,
			Labels:    labels,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func getMilestoneID(ctx context.Context, c *gogithub.Client, owner, repo, milestoneName string) (*int, error) {
	// List all milestones in the repo
	// TODO: This could/should use pagination
	milestones, _, err := c.Issues.ListMilestones(ctx, owner, repo, &gogithub.MilestoneListOptions{
		State: "all",
	})
	if err != nil {
		return nil, err
	}
	// Loop through all milestones, search for one with the right name
	for _, milestone := range milestones {
		// Only consider a milestone with the right name
		if milestone.GetTitle() != milestoneName {
			continue
		}
		// Validate nil to avoid panics
		if milestone.Number == nil {
			return nil, fmt.Errorf("didn't expect milestone Number to be nil: %v", milestone)
		}
		// Return the Milestone number
		return milestone.Number, nil
	}
	return nil, fmt.Errorf("couldn't find milestone with name: %s", milestoneName)
}
