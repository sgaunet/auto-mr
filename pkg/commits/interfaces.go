package commits

// CommitRetriever defines the interface for external git operations (retrieve commits, parse history).
type CommitRetriever interface {
	// GetCommits retrieves all commits from the specified branch.
	// Returns empty slice if branch has no commits.
	// Returns error if branch doesn't exist or git operation fails.
	GetCommits(branch string) ([]Commit, error)
}

// MessageSelector defines the interface for internal selection logic (auto-select, filter, validate).
type MessageSelector interface {
	// GetMessageForMR determines which commit message to use for MR/PR.
	// Handles auto-selection, interactive selection, and manual override.
	// Returns ErrNoCommits if no valid commits exist.
	// Returns ErrSelectionCancelled if user cancels interactive selection.
	GetMessageForMR(commits []Commit, msgFlagValue string) (MessageSelection, error)
}

// SelectionRenderer defines the interface for UI rendering (display list, handle input).
type SelectionRenderer interface {
	// DisplaySelectionPrompt shows interactive commit selection UI.
	// Returns selected commit index.
	// Returns error if user cancels (Ctrl+C).
	DisplaySelectionPrompt(commits []Commit) (int, error)
}
