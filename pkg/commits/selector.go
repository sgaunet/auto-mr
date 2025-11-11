package commits

import (
	"fmt"
	"log/slog"
)

// Selector handles commit message selection logic.
type Selector struct {
	renderer SelectionRenderer
	logger   *slog.Logger
}

// NewSelector creates a new message selector.
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
// Handles auto-selection (single commit) and interactive selection (multiple commits).
// Manual override should be handled before calling this method.
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
