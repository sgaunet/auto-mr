// Package ui provides interactive terminal UI components for label selection.
package ui

import (
	"fmt"
	"log/slog"

	"github.com/AlecAivazis/survey/v2"
	"github.com/sgaunet/auto-mr/internal/logger"
)

// LabelSelector provides interactive label selection functionality.
type LabelSelector struct {
	log *slog.Logger
}

// Label represents a label that can be selected.
type Label interface {
	GetName() string
}

// NewLabelSelector creates a new label selector.
func NewLabelSelector() *LabelSelector {
	return &LabelSelector{log: logger.NoLogger()}
}

// SetLogger sets the logger for the label selector.
func (ls *LabelSelector) SetLogger(logger *slog.Logger) {
	ls.log = logger
}

// SelectLabels presents an interactive multi-select prompt for choosing labels.
func (ls *LabelSelector) SelectLabels(labels []Label, maxSelection int) ([]string, error) {
	if len(labels) == 0 {
		ls.log.Debug("No labels available for selection")
		return []string{}, nil
	}

	options := make([]string, len(labels))
	for i, label := range labels {
		options[i] = label.GetName()
	}

	ls.log.Debug("Prompting user to select labels", "available", len(labels), "max", maxSelection)
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

	ls.log.Debug("Labels selected", "count", len(selected))
	return selected, nil
}

// GitLabLabel represents a GitLab label.
type GitLabLabel struct {
	Name string
}

// GetName returns the label name.
func (gl *GitLabLabel) GetName() string {
	return gl.Name
}

// GitHubLabel represents a GitHub label.
type GitHubLabel struct {
	Name string
}

// GetName returns the label name.
func (gh *GitHubLabel) GetName() string {
	return gh.Name
}