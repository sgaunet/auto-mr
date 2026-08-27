package github

import (
	"context"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/sgaunet/bullets"
)

// APIClient defines the interface for GitHub API operations.
//
// [platform.GitHubAdapter] holds a value of this type rather than the concrete [Client],
// so an implementation can be substituted where the adapter is constructed. Tests use
// that seam two ways: with a fake from testing/mocks, or with a real [Client] pointed
// at an httptest server.
type APIClient interface {
	// SetRepositoryFromURL configures the repository from a git remote URL.
	// Supports both HTTPS and SSH formats.
	SetRepositoryFromURL(ctx context.Context, url string) error

	// ListLabels returns all labels available in the repository.
	ListLabels(ctx context.Context) ([]*Label, error)

	// CreatePullRequest creates a new pull request with the specified parameters.
	// Returns the created pull request or an error if creation fails.
	CreatePullRequest(
		ctx context.Context,
		head, base, title, body string,
		assignees, reviewers, labels []string,
	) (*github.PullRequest, error)

	// GetPullRequestByBranch fetches an existing pull request by head and base branches.
	// Returns errPRNotFound if no matching pull request exists.
	GetPullRequestByBranch(ctx context.Context, head, base string) (*github.PullRequest, error)

	// WaitForWorkflows waits for all workflow runs to complete for the pull request.
	// Returns the overall conclusion (success, failure, etc.) or an error on timeout.
	WaitForWorkflows(ctx context.Context, timeout time.Duration) (string, error)

	// MergePullRequest merges a pull request using the specified merge method.
	// mergeMethod can be "merge", "squash", or "rebase".
	// commitTitle is used as the merge commit message.
	MergePullRequest(ctx context.Context, prNumber int, mergeMethod, commitTitle string) error

	// GetPullRequestsByHead returns all open pull requests for the given head branch.
	GetPullRequestsByHead(ctx context.Context, head string) ([]*github.PullRequest, error)

	// DeleteBranch deletes a branch from the remote repository.
	DeleteBranch(ctx context.Context, branch string) error
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
	// The handle can be updated with new content or converted to success/error.
	InfoHandle(message string) *bullets.BulletHandle

	// SpinnerCircle creates an animated spinner with the given message.
	// Returns a Spinner that can be stopped with Success(), Error(), or Replace().
	SpinnerCircle(ctx context.Context, message string) *bullets.Spinner

	// IncreasePadding increases the indentation level for nested output.
	IncreasePadding()

	// DecreasePadding decreases the indentation level for nested output.
	DecreasePadding()
}

// Ensure Client implements APIClient interface at compile time.
var _ APIClient = (*Client)(nil)
