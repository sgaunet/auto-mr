package forgejo

import "errors"

// Error definitions for Forgejo API operations.
var (
	errTokenRequired    = errors.New("FORGEJO_TOKEN environment variable is required")
	errInvalidURLFormat = errors.New("invalid Forgejo URL format")
	errWorkflowTimeout  = errors.New("timeout waiting for pipeline completion")
	errPRNotFound       = errors.New("no pull request found for branch")
	errPRAlreadyExists  = errors.New("pull request already exists for this branch")

	// ErrTokenRequired is returned when FORGEJO_TOKEN environment variable is missing.
	ErrTokenRequired = errTokenRequired
	// ErrInvalidURLFormat is returned when the Forgejo URL format is invalid.
	ErrInvalidURLFormat = errInvalidURLFormat
	// ErrWorkflowTimeout is returned when waiting for pipeline completion times out.
	ErrWorkflowTimeout = errWorkflowTimeout
	// ErrPRNotFound is returned when no pull request is found for the branch.
	ErrPRNotFound = errPRNotFound
	// ErrPRAlreadyExists is returned when a pull request already exists for the branch.
	ErrPRAlreadyExists = errPRAlreadyExists
)
