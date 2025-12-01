package gitlab

import (
	"time"

	"github.com/sgaunet/bullets"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// APIClient defines the interface for GitLab API operations.
// This interface enables dependency injection and facilitates black box testing
// by allowing mock implementations to replace the actual GitLab API client.
type APIClient interface {
	// SetProjectFromURL configures the project from a git remote URL.
	// Supports both HTTPS and SSH formats.
	SetProjectFromURL(url string) error

	// ListLabels returns all labels available in the project.
	ListLabels() ([]*Label, error)

	// CreateMergeRequest creates a new merge request with the specified parameters.
	// Returns the created merge request or an error if creation fails.
	CreateMergeRequest(
		sourceBranch, targetBranch, title, description, assignee, reviewer string,
		labels []string, squash bool,
	) (*gitlab.MergeRequest, error)

	// GetMergeRequestByBranch fetches an existing merge request by source and target branches.
	// Returns errMRNotFound if no matching merge request exists.
	GetMergeRequestByBranch(sourceBranch, targetBranch string) (*gitlab.MergeRequest, error)

	// WaitForPipeline waits for all pipelines to complete for the merge request.
	// Returns the overall status (success, failed, etc.) or an error on timeout.
	WaitForPipeline(timeout time.Duration) (string, error)

	// ApproveMergeRequest approves a merge request.
	// Returns an error if the approval fails.
	ApproveMergeRequest(mrIID int64) error

	// MergeMergeRequest merges a merge request with optional squash.
	// Returns an error if the merge fails.
	MergeMergeRequest(mrIID int64, squash bool, commitTitle string) error

	// GetMergeRequestsByBranch returns all open merge requests for the given source branch.
	GetMergeRequestsByBranch(sourceBranch string) ([]*gitlab.BasicMergeRequest, error)
}

// StateTracker defines the interface for thread-safe job state management.
// This interface abstracts the jobTracker functionality to enable testing
// of state transitions and display handle management without real API calls.
type StateTracker interface {
	// update processes new jobs, detects state transitions, and updates handles.
	// Returns a list of state transition descriptions for logging/debugging.
	update(newJobs []*Job, logger *bullets.UpdatableLogger) []string

	// getJob retrieves a job by ID with read lock.
	// Returns the Job and a boolean indicating if the job exists.
	getJob(id int64) (*Job, bool)

	// setJob stores a job by ID with write lock.
	setJob(id int64, job *Job)

	// getHandle retrieves a bullet handle by job ID with read lock.
	// Returns the handle and a boolean indicating if it exists.
	getHandle(id int64) (*bullets.BulletHandle, bool)

	// setHandle stores a bullet handle for a job ID with write lock.
	setHandle(id int64, handle *bullets.BulletHandle)

	// getSpinner retrieves a spinner by ID with read lock.
	// Returns the spinner and a boolean indicating if it exists.
	getSpinner(id int64) (*bullets.Spinner, bool)

	// setSpinner stores a spinner for a job ID with write lock.
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
