package github

import "errors"

// Error definitions for GitHub API operations.
var (
	errTokenRequired    = errors.New("GITHUB_TOKEN environment variable is required")
	errInvalidURLFormat = errors.New("invalid GitHub URL format")
	errWorkflowTimeout  = errors.New("timeout waiting for workflow completion")
	errPRNotFound       = errors.New("no pull request found for branch")
	errPRAlreadyExists  = errors.New("pull request already exists for this branch")

	// ErrTokenRequired is returned when GITHUB_TOKEN environment variable is missing.
	ErrTokenRequired = errTokenRequired
	// ErrInvalidURLFormat is returned when the GitHub URL format is invalid.
	ErrInvalidURLFormat = errInvalidURLFormat
	// ErrWorkflowTimeout is returned when waiting for workflow completion times out.
	ErrWorkflowTimeout = errWorkflowTimeout
	// ErrPRNotFound is returned when no pull request is found for the branch.
	ErrPRNotFound = errPRNotFound
	// ErrPRAlreadyExists is returned when a pull request already exists for the branch.
	ErrPRAlreadyExists = errPRAlreadyExists
)
