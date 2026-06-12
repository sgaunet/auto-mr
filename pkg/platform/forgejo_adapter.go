package platform

import (
	"errors"
	"fmt"
	"time"

	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/forgejo"
	"github.com/sgaunet/bullets"
)

// ForgejoAdapter wraps a Forgejo client to implement the [Provider] interface.
// It translates between the platform-agnostic types and the Forgejo-specific API.
type ForgejoAdapter struct {
	client *forgejo.Client
	cfg    config.ForgejoConfig
	log    *bullets.Logger
}

// NewForgejoAdapter creates a new Forgejo adapter.
func NewForgejoAdapter(client *forgejo.Client, cfg config.ForgejoConfig, log *bullets.Logger) *ForgejoAdapter {
	return &ForgejoAdapter{
		client: client,
		cfg:    cfg,
		log:    log,
	}
}

// Initialize sets up the Forgejo repository from a remote URL.
func (a *ForgejoAdapter) Initialize(remoteURL string) error {
	if err := a.client.SetRepositoryFromURL(remoteURL); err != nil {
		return fmt.Errorf("failed to set Forgejo repository: %w", err)
	}
	return nil
}

// ListLabels returns all available labels, converted to platform-agnostic format.
func (a *ForgejoAdapter) ListLabels() ([]Label, error) {
	fjLabels, err := a.client.ListLabels()
	if err != nil {
		return nil, fmt.Errorf("failed to list Forgejo labels: %w", err)
	}

	labels := make([]Label, len(fjLabels))
	for i, l := range fjLabels {
		labels[i] = Label{Name: l.Name}
	}
	return labels, nil
}

// Create creates a new pull request on Forgejo.
func (a *ForgejoAdapter) Create(params CreateParams) (*MergeRequest, error) {
	pr, err := a.client.CreatePullRequest(
		params.SourceBranch, params.TargetBranch,
		params.Title, params.Body,
		a.cfg.Assignee, a.cfg.Reviewer,
		params.Labels,
	)
	if err != nil {
		if errors.Is(err, forgejo.ErrPRAlreadyExists) {
			return nil, fmt.Errorf("%w: %w", ErrAlreadyExists, err)
		}
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	return &MergeRequest{
		ID:           pr.Index,
		WebURL:       pr.HTMLURL,
		SourceBranch: pr.Head.Ref,
	}, nil
}

// GetByBranch fetches an existing pull request by source and target branches.
func (a *ForgejoAdapter) GetByBranch(sourceBranch, targetBranch string) (*MergeRequest, error) {
	pr, err := a.client.GetPullRequestByBranch(sourceBranch, targetBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request by branch: %w", err)
	}

	return &MergeRequest{
		ID:           pr.Index,
		WebURL:       pr.HTMLURL,
		SourceBranch: pr.Head.Ref,
	}, nil
}

// WaitForPipeline waits for Forgejo Actions / commit-status CI completion.
func (a *ForgejoAdapter) WaitForPipeline(timeout time.Duration) (string, error) {
	status, err := a.client.WaitForPipeline(timeout)
	if err != nil {
		return "", fmt.Errorf("failed to wait for Forgejo pipeline: %w", err)
	}
	return status, nil
}

// Approve is a no-op for Forgejo (Forgejo doesn't gate merges on approval).
func (a *ForgejoAdapter) Approve(_ int64) error {
	return nil
}

// Merge merges a Forgejo pull request.
// Branch deletion is handled inside the client via DeleteBranchAfterMerge.
func (a *ForgejoAdapter) Merge(params MergeParams) error {
	if err := a.client.MergePullRequest(params.MRID, params.Squash, params.CommitTitle); err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}
	return nil
}

// PlatformName returns "Forgejo".
func (a *ForgejoAdapter) PlatformName() string {
	return "Forgejo"
}

// PipelineTimeout returns the configured pipeline timeout string.
func (a *ForgejoAdapter) PipelineTimeout() string {
	return a.cfg.PipelineTimeout
}

// Compile-time interface check.
var _ Provider = (*ForgejoAdapter)(nil)
