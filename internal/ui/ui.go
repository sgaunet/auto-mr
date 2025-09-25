package ui

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
)

type LabelSelector struct{}

type Label interface {
	GetName() string
}

func NewLabelSelector() *LabelSelector {
	return &LabelSelector{}
}

func (ls *LabelSelector) SelectLabels(labels []Label, maxSelection int) ([]string, error) {
	if len(labels) == 0 {
		return []string{}, nil
	}

	options := make([]string, len(labels))
	for i, label := range labels {
		options[i] = label.GetName()
	}

	var selected []string
	prompt := &survey.MultiSelect{
		Message: "Choose labels:",
		Options: options,
	}

	if maxSelection > 0 {
		prompt.Help = fmt.Sprintf("(max %d selections)", maxSelection)
	}

	err := survey.AskOne(prompt, &selected)
	if err != nil {
		return nil, fmt.Errorf("failed to get label selection: %w", err)
	}

	// Limit the selection if maxSelection is specified
	if maxSelection > 0 && len(selected) > maxSelection {
		selected = selected[:maxSelection]
	}

	return selected, nil
}

type GitLabLabel struct {
	Name string
}

func (gl *GitLabLabel) GetName() string {
	return gl.Name
}

type GitHubLabel struct {
	Name string
}

func (gh *GitHubLabel) GetName() string {
	return gh.Name
}