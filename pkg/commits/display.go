package commits

import (
	"github.com/AlecAivazis/survey/v2"
)

const (
	// SelectionPageSize is the number of commits to show at once in the selection UI.
	SelectionPageSize = 15
)

// Renderer implements the [SelectionRenderer] interface using the survey library
// for interactive terminal prompts.
type Renderer struct{}

// NewRenderer creates a new selection renderer.
func NewRenderer() *Renderer {
	return &Renderer{}
}

// DisplaySelectionPrompt shows an interactive commit selection UI using survey.Select.
// Each commit is displayed as "[ShortHash] Title" with at most [SelectionPageSize] items visible.
//
// Parameters:
//   - commits: the list of commits to present (must not be empty)
//
// Returns the zero-based index of the selected commit.
// Returns [ErrAllCommitsInvalid] if commits is empty.
// Returns [ErrSelectionCancelled] if the user cancels with Ctrl+C.
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
