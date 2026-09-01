package platform

import (
	"context"
	"time"
)

// Provider defines the unified interface for GitLab, GitHub, and Forgejo operations.
// Implementations are [GitLabAdapter], [GitHubAdapter], and [ForgejoAdapter], created via [NewProvider].
//
// Every method that performs I/O takes a [context.Context] as its first parameter.
// Cancelling it aborts the in-flight request, which is what makes the configured
// pipeline timeout enforceable: without it a stalled response blocks forever
// regardless of the budget.
type Provider interface {
	// Initialize sets up the client from a git remote URL.
	Initialize(ctx context.Context, remoteURL string) error

	// ListLabels returns all available labels.
	ListLabels(ctx context.Context) ([]Label, error)

	// Create creates a new merge/pull request.
	Create(ctx context.Context, params CreateParams) (*MergeRequest, error)

	// GetByBranch fetches an existing merge/pull request by source and target branches.
	GetByBranch(ctx context.Context, sourceBranch, targetBranch string) (*MergeRequest, error)

	// WaitForPipeline waits for CI/CD pipeline or workflow completion.
	// Returns the overall status/conclusion or an error on timeout.
	WaitForPipeline(ctx context.Context, timeout time.Duration) (string, error)

	// Approve approves a merge/pull request.
	// No-op for GitHub (returns nil).
	Approve(ctx context.Context, mrID int64) error

	// Merge merges a merge/pull request.
	// GitHub: also deletes the remote branch internally.
	Merge(ctx context.Context, params MergeParams) error

	// PlatformName returns "GitLab", "GitHub", or "Forgejo".
	PlatformName() string

	// PipelineTimeout returns the config value for timeout resolution.
	PipelineTimeout() string
}
