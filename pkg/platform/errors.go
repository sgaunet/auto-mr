package platform

import "errors"

// Sentinel errors for platform operations.
var (
	// ErrAlreadyExists is returned when a merge/pull request already exists for the branch.
	ErrAlreadyExists = errors.New("merge/pull request already exists for this branch")

	// ErrNotFound is returned when no merge/pull request is found for the branch.
	ErrNotFound = errors.New("no merge/pull request found for branch")
)
