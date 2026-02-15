package commits

import (
	"fmt"
	"log/slog"
)

// Selector handles commit message selection logic with support for
// auto-selection, interactive prompts, and manual override.
// It delegates UI rendering to a [SelectionRenderer] implementation.
//
// Not safe for concurrent use.
type Selector struct {
	renderer SelectionRenderer
	logger   *slog.Logger
}

// NewSelector creates a new message selector with the given renderer.
//
// Parameters:
//   - renderer: the UI renderer for interactive selection (must not be nil)
func NewSelector(renderer SelectionRenderer) *Selector {
	return &Selector{
		renderer: renderer,
		logger:   slog.Default(),
	}
}

// SetLogger sets the logger for the selector.
func (s *Selector) SetLogger(logger *slog.Logger) {
	s.logger = logger
}

// GetMessageForMR determines which commit message to use for MR/PR.
// It applies the following priority:
//  1. Manual override: if msgFlagValue is non-empty, it is parsed and returned.
//  2. Auto-select: if exactly one valid commit exists, it is selected automatically.
//  3. Interactive: if multiple valid commits exist, the renderer prompts the user.
//
// Parameters:
//   - commits: all commits from the branch (will be filtered internally)
//   - msgFlagValue: manual message from --msg flag (empty string to skip)
//
// Returns [ErrAllCommitsInvalid] if no valid commits exist after filtering.
// Returns [ErrSelectionCancelled] if the user cancels interactive selection.
func (s *Selector) GetMessageForMR(commits []Commit, msgFlagValue string) (MessageSelection, error) {
	// Manual override (should be handled by caller, but check here for safety)
	if msgFlagValue != "" {
		title, body := ParseCommitMessage(msgFlagValue)
		return MessageSelection{
			Title:            title,
			Body:             body,
			SourceCommitHash: "",
			SelectionMethod:  SelectionManual,
			ManualOverride:   true,
		}, nil
	}

	// Filter valid commits
	validCommits := FilterValidCommits(commits)

	if len(validCommits) == 0 {
		return MessageSelection{}, ErrAllCommitsInvalid
	}

	// Auto-select single commit
	if len(validCommits) == 1 {
		commit := validCommits[0]
		s.logger.Debug("auto-selecting single commit", "hash", commit.ShortHash, "title", commit.Title)

		return MessageSelection{
			Title:            commit.Title,
			Body:             commit.Body,
			SourceCommitHash: commit.Hash,
			SelectionMethod:  SelectionAuto,
			ManualOverride:   false,
		}, nil
	}

	// Interactive selection for multiple commits
	s.logger.Debug("displaying interactive selection", "commit_count", len(validCommits))

	selectedIndex, err := s.renderer.DisplaySelectionPrompt(validCommits)
	if err != nil {
		return MessageSelection{}, fmt.Errorf("failed to display selection prompt: %w", err)
	}

	if selectedIndex < 0 || selectedIndex >= len(validCommits) {
		return MessageSelection{}, ErrSelectionCancelled
	}

	selectedCommit := validCommits[selectedIndex]
	s.logger.Debug("user selected commit", "hash", selectedCommit.ShortHash, "title", selectedCommit.Title)

	return MessageSelection{
		Title:            selectedCommit.Title,
		Body:             selectedCommit.Body,
		SourceCommitHash: selectedCommit.Hash,
		SelectionMethod:  SelectionInteractive,
		ManualOverride:   false,
	}, nil
}
