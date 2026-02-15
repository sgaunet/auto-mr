package platform

import "time"

// Provider defines the unified interface for GitLab and GitHub operations.
type Provider interface {
	// Initialize sets up the client from a git remote URL.
	Initialize(remoteURL string) error

	// ListLabels returns all available labels.
	ListLabels() ([]Label, error)

	// Create creates a new merge/pull request.
	Create(params CreateParams) (*MergeRequest, error)

	// GetByBranch fetches an existing merge/pull request by source and target branches.
	GetByBranch(sourceBranch, targetBranch string) (*MergeRequest, error)

	// WaitForPipeline waits for CI/CD pipeline or workflow completion.
	// Returns the overall status/conclusion or an error on timeout.
	WaitForPipeline(timeout time.Duration) (string, error)

	// Approve approves a merge/pull request.
	// No-op for GitHub (returns nil).
	Approve(mrID int64) error

	// Merge merges a merge/pull request.
	// GitHub: also deletes the remote branch internally.
	Merge(params MergeParams) error

	// PlatformName returns "GitLab" or "GitHub".
	PlatformName() string

	// PipelineTimeout returns the config value for timeout resolution.
	PipelineTimeout() string
}
