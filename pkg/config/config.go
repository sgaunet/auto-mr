// Package config handles loading and validation of user configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	minPipelineTimeout = 1 * time.Minute
	maxPipelineTimeout = 8 * time.Hour
)

var (
	errConfigNotFound        = errors.New("config file not found")
	errGitLabAssigneeEmpty   = errors.New("gitlab.assignee is required")
	errGitLabReviewerEmpty   = errors.New("gitlab.reviewer is required")
	errGitHubAssigneeEmpty   = errors.New("github.assignee is required")
	errGitHubReviewerEmpty   = errors.New("github.reviewer is required")
	errGitLabAssigneeInvalid = errors.New("gitlab.assignee contains invalid characters")
	errGitLabReviewerInvalid = errors.New("gitlab.reviewer contains invalid characters")
	errGitHubAssigneeInvalid = errors.New("github.assignee contains invalid characters")
	errGitHubReviewerInvalid = errors.New("github.reviewer contains invalid characters")
	errInvalidTimeout        = errors.New("invalid timeout format")
	errTimeoutTooSmall       = errors.New("timeout too small")
	errTimeoutTooLarge       = errors.New("timeout too large")
)

// MinPipelineTimeout is the minimum allowed pipeline timeout (1 minute).
const MinPipelineTimeout = minPipelineTimeout

// MaxPipelineTimeout is the maximum allowed pipeline timeout (8 hours).
const MaxPipelineTimeout = maxPipelineTimeout

// Export for external error checking with errors.Is().
var (
	ErrConfigNotFound        = errConfigNotFound
	ErrGitLabAssigneeEmpty   = errGitLabAssigneeEmpty
	ErrGitLabReviewerEmpty   = errGitLabReviewerEmpty
	ErrGitHubAssigneeEmpty   = errGitHubAssigneeEmpty
	ErrGitHubReviewerEmpty   = errGitHubReviewerEmpty
	ErrGitLabAssigneeInvalid = errGitLabAssigneeInvalid
	ErrGitLabReviewerInvalid = errGitLabReviewerInvalid
	ErrGitHubAssigneeInvalid = errGitHubAssigneeInvalid
	ErrGitHubReviewerInvalid = errGitHubReviewerInvalid
	ErrInvalidTimeout        = errInvalidTimeout
	ErrTimeoutTooSmall       = errTimeoutTooSmall
	ErrTimeoutTooLarge       = errTimeoutTooLarge
)

// Config represents the complete configuration for auto-mr.
type Config struct {
	GitLab GitLabConfig `yaml:"gitlab"`
	GitHub GitHubConfig `yaml:"github"`
}

// GitLabConfig contains GitLab-specific configuration.
type GitLabConfig struct {
	Assignee        string `yaml:"assignee"`
	Reviewer        string `yaml:"reviewer"`
	PipelineTimeout string `yaml:"pipeline_timeout,omitempty"`
}

// GitHubConfig contains GitHub-specific configuration.
type GitHubConfig struct {
	Assignee        string `yaml:"assignee"`
	Reviewer        string `yaml:"reviewer"`
	PipelineTimeout string `yaml:"pipeline_timeout,omitempty"`
}

// Load reads and parses the configuration file from the user's home directory.
func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".config", "auto-mr", "config.yml")

	// #nosec G304 - Reading config from user's home directory is intentional
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errConfigNotFound, configPath)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// Validate checks that all required configuration fields are set and valid.
// It trims whitespace from all fields before validation and performs format checks.
func (c *Config) Validate() error {
	// Trim whitespace from all fields before validation
	c.GitLab.Assignee = strings.TrimSpace(c.GitLab.Assignee)
	c.GitLab.Reviewer = strings.TrimSpace(c.GitLab.Reviewer)
	c.GitLab.PipelineTimeout = strings.TrimSpace(c.GitLab.PipelineTimeout)
	c.GitHub.Assignee = strings.TrimSpace(c.GitHub.Assignee)
	c.GitHub.Reviewer = strings.TrimSpace(c.GitHub.Reviewer)
	c.GitHub.PipelineTimeout = strings.TrimSpace(c.GitHub.PipelineTimeout)

	// Validate GitLab configuration
	if err := validateGitLabConfig(&c.GitLab); err != nil {
		return err
	}

	// Validate GitHub configuration
	if err := validateGitHubConfig(&c.GitHub); err != nil {
		return err
	}

	return nil
}

// validateTimeout validates timeout string format and bounds.
// Empty string is valid (uses default). Returns parsed duration or error.
//
//nolint:unparam // duration return value is used, false positive from linter
func validateTimeout(timeoutStr string, fieldName string) (time.Duration, error) {
	if timeoutStr == "" {
		return 0, nil // Empty is valid (uses default)
	}

	duration, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid duration format '%s'", errInvalidTimeout, timeoutStr)
	}

	if duration < minPipelineTimeout {
		return 0, fmt.Errorf("%w: %s must be at least %v (got %v)",
			errTimeoutTooSmall, fieldName, minPipelineTimeout, duration)
	}

	if duration > maxPipelineTimeout {
		return 0, fmt.Errorf("%w: %s must be at most %v (got %v)",
			errTimeoutTooLarge, fieldName, maxPipelineTimeout, duration)
	}

	return duration, nil
}

// validateGitLabConfig validates GitLab-specific configuration fields.
func validateGitLabConfig(config *GitLabConfig) error {
	if config.Assignee == "" {
		return errGitLabAssigneeEmpty
	}
	if !isValidUsername(config.Assignee) {
		return fmt.Errorf("%w: '%s'", errGitLabAssigneeInvalid, config.Assignee)
	}

	if config.Reviewer == "" {
		return errGitLabReviewerEmpty
	}
	if !isValidUsername(config.Reviewer) {
		return fmt.Errorf("%w: '%s'", errGitLabReviewerInvalid, config.Reviewer)
	}

	if _, err := validateTimeout(config.PipelineTimeout, "gitlab.pipeline_timeout"); err != nil {
		return err
	}

	return nil
}

// validateGitHubConfig validates GitHub-specific configuration fields.
func validateGitHubConfig(config *GitHubConfig) error {
	if config.Assignee == "" {
		return errGitHubAssigneeEmpty
	}
	if !isValidUsername(config.Assignee) {
		return fmt.Errorf("%w: '%s'", errGitHubAssigneeInvalid, config.Assignee)
	}

	if config.Reviewer == "" {
		return errGitHubReviewerEmpty
	}
	if !isValidUsername(config.Reviewer) {
		return fmt.Errorf("%w: '%s'", errGitHubReviewerInvalid, config.Reviewer)
	}

	if _, err := validateTimeout(config.PipelineTimeout, "github.pipeline_timeout"); err != nil {
		return err
	}

	return nil
}

// isValidUsername validates username format for GitLab and GitHub.
// Both platforms have similar restrictions:
// - Alphanumeric characters (a-z, A-Z, 0-9)
// - Hyphens (-) and underscores (_)
// - Cannot start or end with special characters
// - Length: 1-39 characters (conservative, covers both platforms).
func isValidUsername(username string) bool {
	if len(username) == 0 || len(username) > 39 {
		return false
	}

	// First and last must be alphanumeric
	if !isAlphanumeric(rune(username[0])) || !isAlphanumeric(rune(username[len(username)-1])) {
		return false
	}

	// All characters must be alphanumeric, hyphen, or underscore
	for _, ch := range username {
		if !isAlphanumeric(ch) && ch != '-' && ch != '_' {
			return false
		}
	}

	return true
}

// isAlphanumeric checks if a rune is alphanumeric (a-z, A-Z, 0-9).
func isAlphanumeric(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}