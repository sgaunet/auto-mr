package gitlab

import (
	"context"
	"time"

	"github.com/sgaunet/bullets"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// APIClient defines the interface for GitLab API operations.
//
// [platform.GitLabAdapter] holds a value of this type rather than the concrete [Client],
// so an implementation can be substituted where the adapter is constructed. Tests use
// that seam two ways: with a fake from testing/mocks, or with a real [Client] pointed
// at an httptest server.
type APIClient interface {
	// SetProjectFromURL configures the project from a git remote URL.
	// Supports both HTTPS and SSH formats.
	SetProjectFromURL(ctx context.Context, url string) error

	// ListLabels returns all labels available in the project.
	ListLabels(ctx context.Context) ([]*Label, error)

	// CreateMergeRequest creates a new merge request with the specified parameters.
	// Returns the created merge request or an error if creation fails.
	CreateMergeRequest(
		ctx context.Context,
		sourceBranch, targetBranch, title, description, assignee, reviewer string,
		labels []string, squash bool,
	) (*gitlab.MergeRequest, error)

	// GetMergeRequestByBranch fetches an existing merge request by source and target branches.
	// Returns errMRNotFound if no matching merge request exists.
	GetMergeRequestByBranch(ctx context.Context, sourceBranch, targetBranch string) (*gitlab.MergeRequest, error)

	// WaitForPipeline waits for all pipelines to complete for the merge request.
	// Returns the overall status (success, failed, etc.) or an error on timeout.
	WaitForPipeline(ctx context.Context, timeout time.Duration) (string, error)

	// ApproveMergeRequest approves a merge request.
	// Returns an error if the approval fails.
	ApproveMergeRequest(ctx context.Context, mrIID int64) error

	// MergeMergeRequest merges a merge request with optional squash.
	// Returns an error if the merge fails.
	MergeMergeRequest(ctx context.Context, mrIID int64, squash bool, commitTitle string) error

	// GetMergeRequestsByBranch returns all open merge requests for the given source branch.
	GetMergeRequestsByBranch(ctx context.Context, sourceBranch string) ([]*gitlab.BasicMergeRequest, error)
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
