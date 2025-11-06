// Package github provides GitHub API client operations.
package github

import "errors"

// Error definitions for GitHub API operations.
var (
	errTokenRequired    = errors.New("GITHUB_TOKEN environment variable is required")
	errInvalidURLFormat = errors.New("invalid GitHub URL format")
	errWorkflowTimeout  = errors.New("timeout waiting for workflow completion")
	errPRNotFound       = errors.New("no pull request found for branch")

	// Exported errors for testing and external use.
	ErrTokenRequired    = errTokenRequired
	ErrInvalidURLFormat = errInvalidURLFormat
	ErrWorkflowTimeout  = errWorkflowTimeout
	ErrPRNotFound       = errPRNotFound
)
