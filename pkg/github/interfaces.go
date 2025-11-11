package github

import (
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/sgaunet/bullets"
)

// APIClient defines the interface for GitHub API operations.
// This interface enables dependency injection and facilitates black box testing
// by allowing mock implementations to replace the actual GitHub API client.
type APIClient interface {
	// SetRepositoryFromURL configures the repository from a git remote URL.
	// Supports both HTTPS and SSH formats.
	SetRepositoryFromURL(url string) error

	// ListLabels returns all labels available in the repository.
	ListLabels() ([]*Label, error)

	// CreatePullRequest creates a new pull request with the specified parameters.
	// Returns the created pull request or an error if creation fails.
	CreatePullRequest(
		head, base, title, body string,
		assignees, reviewers, labels []string,
	) (*github.PullRequest, error)

	// GetPullRequestByBranch fetches an existing pull request by head and base branches.
	// Returns errPRNotFound if no matching pull request exists.
	GetPullRequestByBranch(head, base string) (*github.PullRequest, error)

	// WaitForWorkflows waits for all workflow runs to complete for the pull request.
	// Returns the overall conclusion (success, failure, etc.) or an error on timeout.
	WaitForWorkflows(timeout time.Duration) (string, error)

	// MergePullRequest merges a pull request using the specified merge method.
	// mergeMethod can be "merge", "squash", or "rebase".
	// commitTitle and commitBody are used as the merge commit message.
	MergePullRequest(prNumber int, mergeMethod, commitTitle, commitBody string) error

	// GetPullRequestsByHead returns all open pull requests for the given head branch.
	GetPullRequestsByHead(head string) ([]*github.PullRequest, error)

	// DeleteBranch deletes a branch from the remote repository.
	DeleteBranch(branch string) error
}

// StateTracker defines the interface for thread-safe job/check state management.
// This interface abstracts the checkTracker functionality to enable testing
// of state transitions and display handle management without real API calls.
type StateTracker interface {
	// update processes new jobs/checks, detects state transitions, and updates handles.
	// Returns a list of state transition descriptions for logging/debugging.
	update(newChecks []*JobInfo, logger *bullets.UpdatableLogger) []string

	// getCheck retrieves a job/check by ID with read lock.
	// Returns the JobInfo and a boolean indicating if the check exists.
	getCheck(id int64) (*JobInfo, bool)

	// setCheck stores a job/check by ID with write lock.
	setCheck(id int64, check *JobInfo)

	// getHandle retrieves a bullet handle by job/check ID with read lock.
	// Returns the handle and a boolean indicating if it exists.
	getHandle(id int64) (*bullets.BulletHandle, bool)

	// setHandle stores a bullet handle for a job/check ID with write lock.
	setHandle(id int64, handle *bullets.BulletHandle)

	// getSpinner retrieves a spinner by ID with read lock.
	// Returns the spinner and a boolean indicating if it exists.
	getSpinner(id int64) (*bullets.Spinner, bool)

	// setSpinner stores a spinner for a job/check ID with write lock.
	setSpinner(id int64, spinner *bullets.Spinner)

	// deleteSpinner removes a spinner with write lock.
	// Stops the animation before deletion.
	deleteSpinner(id int64)
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
	SpinnerCircle(message string) *bullets.Spinner

	// IncreasePadding increases the indentation level for nested output.
	IncreasePadding()

	// DecreasePadding decreases the indentation level for nested output.
	DecreasePadding()
}

// Ensure Client implements APIClient interface at compile time.
var _ APIClient = (*Client)(nil)
