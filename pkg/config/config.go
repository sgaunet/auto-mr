package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GitLab GitLabConfig `yaml:"gitlab"`
	GitHub GitHubConfig `yaml:"github"`
}

type GitLabConfig struct {
	Assignee string `yaml:"assignee"`
	Reviewer string `yaml:"reviewer"`
}

type GitHubConfig struct {
	Assignee string `yaml:"assignee"`
	Reviewer string `yaml:"reviewer"`
}

func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".config", "auto-mr", "config.yml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("config file not found: %s", configPath)
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

func (c *Config) Validate() error {
	if c.GitLab.Assignee == "" || c.GitLab.Reviewer == "" {
		return fmt.Errorf("assignee or reviewer is not set for gitlab")
	}

	if c.GitHub.Assignee == "" || c.GitHub.Reviewer == "" {
		return fmt.Errorf("assignee or reviewer is not set for github")
	}

	return nil
}