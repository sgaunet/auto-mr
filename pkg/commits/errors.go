package commits

import "errors"

var (
	// ErrNoCommits is returned when no commits are found on the branch.
	ErrNoCommits = errors.New("no commits found on branch")

	// ErrAllCommitsInvalid is returned when all commits have empty messages or are merge commits.
	ErrAllCommitsInvalid = errors.New("all commits have empty messages")

	// ErrSelectionCancelled is returned when user cancels the interactive commit selection.
	ErrSelectionCancelled = errors.New("commit selection cancelled by user")

	// ErrMultipleCommitsFound is returned when multiple commits exist and interactive selection is needed.
	ErrMultipleCommitsFound = errors.New("multiple commits found")
)
