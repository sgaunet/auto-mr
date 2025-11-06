// Package gitlab provides GitLab API client operations.
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

	// Exported errors for testing and external use.
	ErrTokenRequired    = errTokenRequired
	ErrInvalidURLFormat = errInvalidURLFormat
	ErrAssigneeNotFound = errAssigneeNotFound
	ErrReviewerNotFound = errReviewerNotFound
	ErrPipelineTimeout  = errPipelineTimeout
	ErrMRNotFound       = errMRNotFound
)
