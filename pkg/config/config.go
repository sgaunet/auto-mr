// Package config handles loading and validation of user configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var (
	errConfigNotFound        = errors.New("config file not found")
	errGitLabConfigIncomplete = errors.New("assignee or reviewer is not set for gitlab")
	errGitHubConfigIncomplete = errors.New("assignee or reviewer is not set for github")
)

// Config represents the complete configuration for auto-mr.
type Config struct {
	GitLab GitLabConfig `yaml:"gitlab"`
	GitHub GitHubConfig `yaml:"github"`
}

// GitLabConfig contains GitLab-specific configuration.
type GitLabConfig struct {
	Assignee string `yaml:"assignee"`
	Reviewer string `yaml:"reviewer"`
}

// GitHubConfig contains GitHub-specific configuration.
type GitHubConfig struct {
	Assignee string `yaml:"assignee"`
	Reviewer string `yaml:"reviewer"`
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

// Validate checks that all required configuration fields are set.
func (c *Config) Validate() error {
	if c.GitLab.Assignee == "" || c.GitLab.Reviewer == "" {
		return errGitLabConfigIncomplete
	}

	if c.GitHub.Assignee == "" || c.GitHub.Reviewer == "" {
		return errGitHubConfigIncomplete
	}

	return nil
}