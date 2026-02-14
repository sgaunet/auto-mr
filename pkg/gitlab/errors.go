package gitlab

import "errors"

// Error definitions for GitLab API operations.
var (
	errTokenRequired    = errors.New("GITLAB_TOKEN environment variable is required")
	errInvalidURLFormat = errors.New("invalid GitLab URL format")
	errAssigneeNotFound = errors.New("failed to find assignee user")
	errReviewerNotFound = errors.New("failed to find reviewer user")
	errPipelineTimeout  = errors.New("timeout waiting for pipeline completion")
	errMRNotFound       = errors.New("no merge request found for branch")
	errMRAlreadyExists  = errors.New("merge request already exists for this branch")

	// ErrTokenRequired is returned when GITLAB_TOKEN environment variable is missing.
	ErrTokenRequired = errTokenRequired
	// ErrInvalidURLFormat is returned when the GitLab URL format is invalid.
	ErrInvalidURLFormat = errInvalidURLFormat
	// ErrAssigneeNotFound is returned when the assignee user cannot be found.
	ErrAssigneeNotFound = errAssigneeNotFound
	// ErrReviewerNotFound is returned when the reviewer user cannot be found.
	ErrReviewerNotFound = errReviewerNotFound
	// ErrPipelineTimeout is returned when waiting for pipeline completion times out.
	ErrPipelineTimeout = errPipelineTimeout
	// ErrMRNotFound is returned when no merge request is found for the branch.
	ErrMRNotFound = errMRNotFound
	// ErrMRAlreadyExists is returned when a merge request already exists for the branch.
	ErrMRAlreadyExists = errMRAlreadyExists
)
