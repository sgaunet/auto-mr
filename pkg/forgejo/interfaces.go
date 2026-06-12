package forgejo

import (
	"context"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/sgaunet/bullets"
)

// APIClient defines the interface for Forgejo API operations.
// This interface enables dependency injection and facilitates black box testing
// by allowing mock implementations to replace the actual Forgejo API client.
type APIClient interface {
	// SetRepositoryFromURL configures the repository from a git remote URL.
	// Supports both HTTPS and SSH formats.
	SetRepositoryFromURL(url string) error

	// ListLabels returns all labels available in the repository.
	ListLabels() ([]Label, error)

	// CreatePullRequest creates a new pull request with the specified parameters.
	// Returns the created pull request or an error if creation fails.
	CreatePullRequest(
		head, base, title, body, assignee, reviewer string,
		labels []string,
	) (*gitea.PullRequest, error)

	// GetPullRequestByBranch fetches an existing pull request by head and base branches.
	// Returns ErrPRNotFound if no matching pull request exists.
	GetPullRequestByBranch(head, base string) (*gitea.PullRequest, error)

	// WaitForPipeline waits for all commit statuses to complete for the pull request.
	// Returns the overall result ("success", "failure", "error") or an error on timeout.
	WaitForPipeline(timeout time.Duration) (string, error)

	// MergePullRequest merges a pull request using the specified strategy.
	// index is the PR index (number). squash controls merge style.
	// commitTitle is used as the merge commit message.
	MergePullRequest(index int64, squash bool, commitTitle string) error
}

// DisplayRenderer defines the interface for UI rendering operations.
// This interface abstracts the bullets.Logger and bullets.UpdatableLogger
// functionality to enable testing of display logic without actual terminal output.
type DisplayRenderer interface {
	// Info logs an informational message.
	Info(message string)

	// Debug logs a debug message.
	Debug(message string)

	// Error logs an error message.
	Error(message string)

	// Success logs a success message.
	Success(message string)

	// InfoHandle creates an updatable handle for an info message.
	InfoHandle(message string) *bullets.BulletHandle

	// SpinnerCircle creates an animated spinner with the given message.
	SpinnerCircle(ctx context.Context, message string) *bullets.Spinner

	// IncreasePadding increases the indentation level for nested output.
	IncreasePadding()

	// DecreasePadding decreases the indentation level for nested output.
	DecreasePadding()
}

// Ensure Client implements APIClient interface at compile time.
var _ APIClient = (*Client)(nil)
