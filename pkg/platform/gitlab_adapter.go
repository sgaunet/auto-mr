package platform

import (
	"errors"
	"fmt"
	"time"

	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/sgaunet/bullets"
)

// GitLabAdapter wraps a GitLab client to implement the Provider interface.
type GitLabAdapter struct {
	client *gitlab.Client
	cfg    config.GitLabConfig
}

// NewGitLabAdapter creates a new GitLab adapter.
func NewGitLabAdapter(client *gitlab.Client, cfg config.GitLabConfig, _ *bullets.Logger) *GitLabAdapter {
	return &GitLabAdapter{
		client: client,
		cfg:    cfg,
	}
}

// Initialize sets up the GitLab project from a remote URL.
func (a *GitLabAdapter) Initialize(remoteURL string) error {
	if err := a.client.SetProjectFromURL(remoteURL); err != nil {
		return fmt.Errorf("failed to set GitLab project: %w", err)
	}
	return nil
}

// ListLabels returns all available labels, converted to platform-agnostic format.
func (a *GitLabAdapter) ListLabels() ([]Label, error) {
	glLabels, err := a.client.ListLabels()
	if err != nil {
		return nil, fmt.Errorf("failed to list GitLab labels: %w", err)
	}

	labels := make([]Label, len(glLabels))
	for i, l := range glLabels {
		labels[i] = Label{Name: l.Name}
	}
	return labels, nil
}

// Create creates a new merge request on GitLab.
func (a *GitLabAdapter) Create(params CreateParams) (*MergeRequest, error) {
	mr, err := a.client.CreateMergeRequest(
		params.SourceBranch, params.TargetBranch,
		params.Title, params.Body,
		a.cfg.Assignee, a.cfg.Reviewer,
		params.Labels, params.Squash,
	)
	if err != nil {
		if errors.Is(err, gitlab.ErrMRAlreadyExists) {
			return nil, fmt.Errorf("%w: %w", ErrAlreadyExists, err)
		}
		return nil, fmt.Errorf("failed to create merge request: %w", err)
	}

	return &MergeRequest{
		ID:           mr.IID,
		WebURL:       mr.WebURL,
		SourceBranch: mr.SourceBranch,
	}, nil
}

// GetByBranch fetches an existing merge request by source and target branches.
func (a *GitLabAdapter) GetByBranch(sourceBranch, targetBranch string) (*MergeRequest, error) {
	mr, err := a.client.GetMergeRequestByBranch(sourceBranch, targetBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request by branch: %w", err)
	}

	return &MergeRequest{
		ID:           mr.IID,
		WebURL:       mr.WebURL,
		SourceBranch: mr.SourceBranch,
	}, nil
}

// WaitForPipeline waits for GitLab pipeline completion.
func (a *GitLabAdapter) WaitForPipeline(timeout time.Duration) (string, error) {
	status, err := a.client.WaitForPipeline(timeout)
	if err != nil {
		return "", fmt.Errorf("failed to wait for GitLab pipeline: %w", err)
	}
	return status, nil
}

// Approve approves a GitLab merge request.
func (a *GitLabAdapter) Approve(mrID int64) error {
	if err := a.client.ApproveMergeRequest(mrID); err != nil {
		return fmt.Errorf("failed to approve merge request: %w", err)
	}
	return nil
}

// Merge merges a GitLab merge request.
// Branch deletion is handled by GitLab's RemoveSourceBranch flag set during creation.
func (a *GitLabAdapter) Merge(params MergeParams) error {
	if err := a.client.MergeMergeRequest(params.MRID, params.Squash, params.CommitTitle); err != nil {
		return fmt.Errorf("failed to merge MR: %w", err)
	}
	return nil
}

// PlatformName returns "GitLab".
func (a *GitLabAdapter) PlatformName() string {
	return "GitLab"
}

// PipelineTimeout returns the configured pipeline timeout string.
func (a *GitLabAdapter) PipelineTimeout() string {
	return a.cfg.PipelineTimeout
}

// Compile-time interface check.
var _ Provider = (*GitLabAdapter)(nil)
