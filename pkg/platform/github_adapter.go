package platform

import (
	"errors"
	"fmt"
	"time"

	"github.com/sgaunet/auto-mr/pkg/config"
	ghclient "github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/bullets"
)

// GitHubAdapter wraps a GitHub client to implement the [Provider] interface.
// It translates between the platform-agnostic types and the GitHub-specific API.
type GitHubAdapter struct {
	client *ghclient.Client
	cfg    config.GitHubConfig
	log    *bullets.Logger
}

// NewGitHubAdapter creates a new GitHub adapter.
func NewGitHubAdapter(client *ghclient.Client, cfg config.GitHubConfig, log *bullets.Logger) *GitHubAdapter {
	return &GitHubAdapter{
		client: client,
		cfg:    cfg,
		log:    log,
	}
}

// Initialize sets up the GitHub repository from a remote URL.
func (a *GitHubAdapter) Initialize(remoteURL string) error {
	if err := a.client.SetRepositoryFromURL(remoteURL); err != nil {
		return fmt.Errorf("failed to set GitHub repository: %w", err)
	}
	return nil
}

// ListLabels returns all available labels, converted to platform-agnostic format.
func (a *GitHubAdapter) ListLabels() ([]Label, error) {
	ghLabels, err := a.client.ListLabels()
	if err != nil {
		return nil, fmt.Errorf("failed to list GitHub labels: %w", err)
	}

	labels := make([]Label, len(ghLabels))
	for i, l := range ghLabels {
		labels[i] = Label{Name: l.Name}
	}
	return labels, nil
}

// Create creates a new pull request on GitHub.
func (a *GitHubAdapter) Create(params CreateParams) (*MergeRequest, error) {
	pr, err := a.client.CreatePullRequest(
		params.SourceBranch, params.TargetBranch,
		params.Title, params.Body,
		[]string{a.cfg.Assignee},
		[]string{a.cfg.Reviewer},
		params.Labels,
	)
	if err != nil {
		if errors.Is(err, ghclient.ErrPRAlreadyExists) {
			return nil, fmt.Errorf("%w: %w", ErrAlreadyExists, err)
		}
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	return &MergeRequest{
		ID:           int64(*pr.Number),
		WebURL:       *pr.HTMLURL,
		SourceBranch: *pr.Head.Ref,
	}, nil
}

// GetByBranch fetches an existing pull request by source and target branches.
func (a *GitHubAdapter) GetByBranch(sourceBranch, targetBranch string) (*MergeRequest, error) {
	pr, err := a.client.GetPullRequestByBranch(sourceBranch, targetBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request by branch: %w", err)
	}

	return &MergeRequest{
		ID:           int64(*pr.Number),
		WebURL:       *pr.HTMLURL,
		SourceBranch: *pr.Head.Ref,
	}, nil
}

// WaitForPipeline waits for GitHub workflow completion.
func (a *GitHubAdapter) WaitForPipeline(timeout time.Duration) (string, error) {
	conclusion, err := a.client.WaitForWorkflows(timeout)
	if err != nil {
		return "", fmt.Errorf("failed to wait for GitHub workflows: %w", err)
	}
	return conclusion, nil
}

// Approve is a no-op for GitHub (GitHub doesn't require self-approval).
func (a *GitHubAdapter) Approve(_ int64) error {
	return nil
}

// Merge merges a GitHub pull request and deletes the remote branch.
func (a *GitHubAdapter) Merge(params MergeParams) error {
	mergeMethod := ghclient.GetMergeMethod(params.Squash)
	if err := a.client.MergePullRequest(int(params.MRID), mergeMethod, params.CommitTitle); err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	// Delete remote branch after successful merge (matching shell script behavior)
	a.log.Infof("Deleting remote branch: %s", params.SourceBranch)
	if err := a.client.DeleteBranch(params.SourceBranch); err != nil {
		a.log.Warnf("Failed to delete remote branch: %v", err)
		// Don't fail the entire operation if branch deletion fails
	}

	return nil
}

// PlatformName returns "GitHub".
func (a *GitHubAdapter) PlatformName() string {
	return "GitHub"
}

// PipelineTimeout returns the configured pipeline timeout string.
func (a *GitHubAdapter) PipelineTimeout() string {
	return a.cfg.PipelineTimeout
}

// Compile-time interface check.
var _ Provider = (*GitHubAdapter)(nil)
