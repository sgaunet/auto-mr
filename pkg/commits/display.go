package commits

import (
	"github.com/AlecAivazis/survey/v2"
)

const (
	// SelectionPageSize is the number of commits to show at once in the selection UI.
	SelectionPageSize = 15
)

// Renderer implements the SelectionRenderer interface using survey library.
type Renderer struct{}

// NewRenderer creates a new selection renderer.
func NewRenderer() *Renderer {
	return &Renderer{}
}

// DisplaySelectionPrompt shows interactive commit selection UI.
// Returns selected commit index.
// Returns error if user cancels (Ctrl+C).
func (r *Renderer) DisplaySelectionPrompt(commits []Commit) (int, error) {
	if len(commits) == 0 {
		return -1, ErrAllCommitsInvalid
	}

	// Format commits for display: "[ShortHash] TitleTruncated(80)"
	options := make([]string, len(commits))
	for i, c := range commits {
		options[i] = c.FormattedForDisplay()
	}

	prompt := &survey.Select{
		Message: "Select commit message for MR/PR:",
		Options: options,
		PageSize: SelectionPageSize,
	}

	var selectedIndex int
	err := survey.AskOne(prompt, &selectedIndex)
	if err != nil {
		return -1, ErrSelectionCancelled
	}

	return selectedIndex, nil
}
