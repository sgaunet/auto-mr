package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sgaunet/auto-mr/pkg/config"
)

// YAML fixtures for Load() tests.
const (
	validConfigYAML = `
gitlab:
  assignee: john-doe
  reviewer: jane-smith
github:
  assignee: bob-jones
  reviewer: alice-wilson
`

	validConfigWithWhitespace = `
gitlab:
  assignee: "  john-doe  "
  reviewer: "  jane-smith  "
github:
  assignee: "  bob-jones  "
  reviewer: "  alice-wilson  "
`

	validMinimalUsernames = `
gitlab:
  assignee: a
  reviewer: b
github:
  assignee: c
  reviewer: d
`

	validMaxLengthUsernames = `
gitlab:
  assignee: abcdefghijklmnopqrstuvwxyz1234567890123
  reviewer: zyxwvutsrqponmlkjihgfedcba9876543210987
github:
  assignee: ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123
  reviewer: ZYXWVUTSRQPONMLKJIHGFEDCBA9876543210987
`

	validConfigWithComments = `
# Auto-MR Configuration File
gitlab:
  # GitLab user assignments
  assignee: john-doe
  reviewer: jane-smith
github:
  # GitHub user assignments
  assignee: bob-jones
  reviewer: alice-wilson
`

	malformedYAMLIndentation = `
gitlab:
assignee: john
  reviewer: jane
github:
  assignee: bob
  reviewer: alice
`

	malformedYAMLTabs = `
gitlab:
	assignee: john
	reviewer: jane
github:
	assignee: bob
	reviewer: alice
`

	malformedYAMLUnclosedQuote = `
gitlab:
  assignee: "john
  reviewer: jane
github:
  assignee: bob
  reviewer: alice
`

	emptyYAML = ``

	whitespaceOnlyYAML = `


`

	configMissingGitLabAssignee = `
gitlab:
  assignee: ""
  reviewer: jane-smith
github:
  assignee: bob-jones
  reviewer: alice-wilson
`

	configMissingGitLabReviewer = `
gitlab:
  assignee: john-doe
  reviewer: ""
github:
  assignee: bob-jones
  reviewer: alice-wilson
`

	configMissingGitHubAssignee = `
gitlab:
  assignee: john-doe
  reviewer: jane-smith
github:
  assignee: ""
  reviewer: alice-wilson
`

	configMissingGitHubReviewer = `
gitlab:
  assignee: john-doe
  reviewer: jane-smith
github:
  assignee: bob-jones
  reviewer: ""
`

	configInvalidGitLabAssigneeFormat = `
gitlab:
  assignee: -invalid-start
  reviewer: jane-smith
github:
  assignee: bob-jones
  reviewer: alice-wilson
`

	configInvalidGitLabReviewerFormat = `
gitlab:
  assignee: john-doe
  reviewer: invalid.period
github:
  assignee: bob-jones
  reviewer: alice-wilson
`

	configInvalidGitHubAssigneeFormat = `
gitlab:
  assignee: john-doe
  reviewer: jane-smith
github:
  assignee: bob@invalid
  reviewer: alice-wilson
`

	configInvalidGitHubReviewerFormat = `
gitlab:
  assignee: john-doe
  reviewer: jane-smith
github:
  assignee: bob-jones
  reviewer: alice_
`

	configUsernameTooLong = `
gitlab:
  assignee: abcdefghijklmnopqrstuvwxyz12345678901234
  reviewer: jane-smith
github:
  assignee: bob-jones
  reviewer: alice-wilson
`

	configUnicodeCharacters = `
gitlab:
  assignee: user名前
  reviewer: jane-smith
github:
  assignee: bob-jones
  reviewer: alice-wilson
`
)

// setupTestConfig creates a temporary home directory with a config file.
// It uses t.TempDir() for automatic cleanup and t.Setenv() to redirect $HOME.
func setupTestConfig(t *testing.T, configContent string) string {
	t.Helper()

	// Create temporary home directory (auto-cleaned after test)
	tmpHome := t.TempDir()

	// Set $HOME to temporary directory (auto-restored after test)
	t.Setenv("HOME", tmpHome)

	// Create config directory structure
	configDir := filepath.Join(tmpHome, ".config", "auto-mr")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	// Write config file
	configPath := filepath.Join(configDir, "config.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	return configPath
}

// TestValidateGitLabAssignee tests GitLab assignee field validation.
func TestValidateGitLabAssignee(t *testing.T) {
	tests := []struct {
		name      string
		assignee  string
		reviewer  string
		wantError error
	}{
		// Valid assignee tests
		{"valid assignee", "john-doe", "reviewer", nil},
		{"valid with underscore", "john_doe", "reviewer", nil},
		{"valid with numbers", "user123", "reviewer", nil},
		{"valid single char", "a", "reviewer", nil},
		{"valid all caps", "JOHNDOE", "reviewer", nil},
		{"valid mixed case", "JohnDoe", "reviewer", nil},
		{"valid max length 39", "abcdefghijklmnopqrstuvwxyz1234567890123", "reviewer", nil},

		// Empty assignee tests
		{"empty assignee", "", "reviewer", config.ErrGitLabAssigneeEmpty},
		{"whitespace-only assignee", "   ", "reviewer", config.ErrGitLabAssigneeEmpty},
		{"tab-only assignee", "\t\t", "reviewer", config.ErrGitLabAssigneeEmpty},
		{"newline assignee", "\n", "reviewer", config.ErrGitLabAssigneeEmpty},

		// Invalid format tests
		{"starts with hyphen", "-john", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"ends with hyphen", "john-", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"starts with underscore", "_john", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"ends with underscore", "john_", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"contains @", "john@doe", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"contains period", "john.doe", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"contains space", "john doe", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"contains special chars", "john#doe", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"too long 40 chars", "abcdefghijklmnopqrstuvwxyz12345678901234", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"too long 50 chars", "abcdefghijklmnopqrstuvwxyz123456789012345678901234", "reviewer", config.ErrGitLabAssigneeInvalid},
		{"consecutive hyphens", "john--doe", "reviewer", nil}, // This is actually valid
		{"consecutive underscores", "john__doe", "reviewer", nil}, // This is actually valid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				GitLab: config.GitLabConfig{Assignee: tt.assignee, Reviewer: tt.reviewer},
				GitHub: config.GitHubConfig{Assignee: "valid", Reviewer: "valid"},
			}
			err := cfg.Validate()

			if tt.wantError != nil {
				if err == nil {
					t.Errorf("Expected error %v, got nil", tt.wantError)
				} else if !errors.Is(err, tt.wantError) {
					t.Errorf("Expected error %v, got %v", tt.wantError, err)
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestValidateGitLabReviewer tests GitLab reviewer field validation.
func TestValidateGitLabReviewer(t *testing.T) {
	tests := []struct {
		name      string
		assignee  string
		reviewer  string
		wantError error
	}{
		// Valid reviewer tests
		{"valid reviewer", "assignee", "jane-doe", nil},
		{"valid with underscore", "assignee", "jane_smith", nil},
		{"valid with numbers", "assignee", "reviewer123", nil},
		{"valid single char", "assignee", "r", nil},

		// Empty reviewer tests
		{"empty reviewer", "assignee", "", config.ErrGitLabReviewerEmpty},
		{"whitespace-only reviewer", "assignee", "   ", config.ErrGitLabReviewerEmpty},
		{"tab-only reviewer", "assignee", "\t", config.ErrGitLabReviewerEmpty},

		// Invalid format tests
		{"starts with hyphen", "assignee", "-jane", config.ErrGitLabReviewerInvalid},
		{"ends with hyphen", "assignee", "jane-", config.ErrGitLabReviewerInvalid},
		{"contains @", "assignee", "jane@smith", config.ErrGitLabReviewerInvalid},
		{"contains period", "assignee", "jane.smith", config.ErrGitLabReviewerInvalid},
		{"contains space", "assignee", "jane smith", config.ErrGitLabReviewerInvalid},
		{"too long", "assignee", "abcdefghijklmnopqrstuvwxyz12345678901234", config.ErrGitLabReviewerInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				GitLab: config.GitLabConfig{Assignee: tt.assignee, Reviewer: tt.reviewer},
				GitHub: config.GitHubConfig{Assignee: "valid", Reviewer: "valid"},
			}
			err := cfg.Validate()

			if tt.wantError != nil {
				if err == nil {
					t.Errorf("Expected error %v, got nil", tt.wantError)
				} else if !errors.Is(err, tt.wantError) {
					t.Errorf("Expected error %v, got %v", tt.wantError, err)
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestValidateGitHubAssignee tests GitHub assignee field validation.
func TestValidateGitHubAssignee(t *testing.T) {
	tests := []struct {
		name      string
		assignee  string
		reviewer  string
		wantError error
	}{
		// Valid assignee tests
		{"valid assignee", "bob-smith", "reviewer", nil},
		{"valid with underscore", "bob_jones", "reviewer", nil},
		{"valid with numbers", "bob123", "reviewer", nil},
		{"valid single char", "b", "reviewer", nil},

		// Empty assignee tests
		{"empty assignee", "", "reviewer", config.ErrGitHubAssigneeEmpty},
		{"whitespace-only assignee", "  ", "reviewer", config.ErrGitHubAssigneeEmpty},
		{"mixed whitespace", " \t ", "reviewer", config.ErrGitHubAssigneeEmpty},

		// Invalid format tests
		{"starts with hyphen", "-bob", "reviewer", config.ErrGitHubAssigneeInvalid},
		{"ends with hyphen", "bob-", "reviewer", config.ErrGitHubAssigneeInvalid},
		{"starts with underscore", "_bob", "reviewer", config.ErrGitHubAssigneeInvalid},
		{"ends with underscore", "bob_", "reviewer", config.ErrGitHubAssigneeInvalid},
		{"contains @", "bob@smith", "reviewer", config.ErrGitHubAssigneeInvalid},
		{"contains period", "bob.smith", "reviewer", config.ErrGitHubAssigneeInvalid},
		{"contains space", "bob smith", "reviewer", config.ErrGitHubAssigneeInvalid},
		{"too long", "abcdefghijklmnopqrstuvwxyz12345678901234", "reviewer", config.ErrGitHubAssigneeInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				GitLab: config.GitLabConfig{Assignee: "valid", Reviewer: "valid"},
				GitHub: config.GitHubConfig{Assignee: tt.assignee, Reviewer: tt.reviewer},
			}
			err := cfg.Validate()

			if tt.wantError != nil {
				if err == nil {
					t.Errorf("Expected error %v, got nil", tt.wantError)
				} else if !errors.Is(err, tt.wantError) {
					t.Errorf("Expected error %v, got %v", tt.wantError, err)
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestValidateGitHubReviewer tests GitHub reviewer field validation.
func TestValidateGitHubReviewer(t *testing.T) {
	tests := []struct {
		name      string
		assignee  string
		reviewer  string
		wantError error
	}{
		// Valid reviewer tests
		{"valid reviewer", "assignee", "alice-review", nil},
		{"valid with underscore", "assignee", "alice_reviewer", nil},
		{"valid with numbers", "assignee", "alice123", nil},
		{"valid single char", "assignee", "a", nil},

		// Empty reviewer tests
		{"empty reviewer", "assignee", "", config.ErrGitHubReviewerEmpty},
		{"whitespace-only reviewer", "assignee", "   ", config.ErrGitHubReviewerEmpty},

		// Invalid format tests
		{"starts with hyphen", "assignee", "-alice", config.ErrGitHubReviewerInvalid},
		{"ends with hyphen", "assignee", "alice-", config.ErrGitHubReviewerInvalid},
		{"contains @", "assignee", "alice@review", config.ErrGitHubReviewerInvalid},
		{"contains period", "assignee", "alice.review", config.ErrGitHubReviewerInvalid},
		{"contains space", "assignee", "alice review", config.ErrGitHubReviewerInvalid},
		{"too long", "assignee", "abcdefghijklmnopqrstuvwxyz12345678901234", config.ErrGitHubReviewerInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				GitLab: config.GitLabConfig{Assignee: "valid", Reviewer: "valid"},
				GitHub: config.GitHubConfig{Assignee: tt.assignee, Reviewer: tt.reviewer},
			}
			err := cfg.Validate()

			if tt.wantError != nil {
				if err == nil {
					t.Errorf("Expected error %v, got nil", tt.wantError)
				} else if !errors.Is(err, tt.wantError) {
					t.Errorf("Expected error %v, got %v", tt.wantError, err)
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestWhitespaceTrimming tests that whitespace is properly trimmed before validation.
func TestWhitespaceTrimming(t *testing.T) {
	tests := []struct {
		name     string
		assignee string
		reviewer string
		wantErr  bool
	}{
		{"leading spaces assignee", "  john", "jane", false},
		{"trailing spaces assignee", "john  ", "jane", false},
		{"both sides spaces assignee", "  john  ", "jane", false},
		{"leading spaces reviewer", "john", "  jane", false},
		{"trailing spaces reviewer", "john", "jane  ", false},
		{"both sides spaces reviewer", "john", "  jane  ", false},
		{"tabs assignee", "\tjohn\t", "jane", false},
		{"tabs reviewer", "john", "\tjane\t", false},
		{"mixed whitespace", " \t john \t ", " \t jane \t ", false},
		{"all fields with whitespace", "  john  ", "  jane  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				GitLab: config.GitLabConfig{Assignee: tt.assignee, Reviewer: tt.reviewer},
				GitHub: config.GitHubConfig{Assignee: "valid", Reviewer: "valid"},
			}
			err := cfg.Validate()

			if tt.wantErr && err == nil {
				t.Error("Expected error, got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify whitespace was trimmed
			if !tt.wantErr {
				if cfg.GitLab.Assignee != "john" && cfg.GitLab.Assignee != "valid" {
					t.Errorf("Expected assignee to be trimmed, got '%s'", cfg.GitLab.Assignee)
				}
				if cfg.GitLab.Reviewer != "jane" && cfg.GitLab.Reviewer != "valid" {
					t.Errorf("Expected reviewer to be trimmed, got '%s'", cfg.GitLab.Reviewer)
				}
			}
		})
	}
}

// TestEdgeCaseUsernames tests edge case username scenarios.
func TestEdgeCaseUsernames(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		wantValid bool
	}{
		// Valid edge cases
		{"single char lowercase", "a", true},
		{"single char uppercase", "Z", true},
		{"single digit", "9", true},
		{"exactly 39 chars", "abcdefghijklmnopqrstuvwxyz1234567890123", true},
		{"numbers only", "123456", true},
		{"alternating hyphen", "a-b-c-d", true},
		{"alternating underscore", "a_b_c_d", true},
		{"mixed separators", "a-b_c-d_e", true},
		{"multiple consecutive hyphens", "abc---def", true},
		{"multiple consecutive underscores", "abc___def", true},

		// Invalid edge cases
		{"empty", "", false},
		{"exactly 40 chars", "abcdefghijklmnopqrstuvwxyz12345678901234", false},
		{"exactly 50 chars", "abcdefghijklmnopqrstuvwxyz123456789012345678901234", false},
		{"starts with hyphen", "-abc", false},
		{"ends with hyphen", "abc-", false},
		{"starts with underscore", "_abc", false},
		{"ends with underscore", "abc_", false},
		{"only hyphen", "-", false},
		{"only underscore", "_", false},
		{"hyphen and underscore", "-_", false},
		{"unicode chars", "user名前", false},
		{"emoji", "user😀", false},
		{"exclamation", "user!", false},
		{"question mark", "user?", false},
		{"forward slash", "user/name", false},
		{"backslash", "user\\name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				GitLab: config.GitLabConfig{Assignee: tt.username, Reviewer: "valid"},
				GitHub: config.GitHubConfig{Assignee: "valid", Reviewer: "valid"},
			}
			err := cfg.Validate()

			if tt.wantValid && err != nil {
				t.Errorf("Expected username '%s' to be valid, got error: %v", tt.username, err)
			} else if !tt.wantValid && err == nil {
				t.Errorf("Expected username '%s' to be invalid, got no error", tt.username)
			}
		})
	}
}

// TestValidationOrder tests that GitLab fields are validated before GitHub fields.
func TestValidationOrder(t *testing.T) {
	// Test that GitLab assignee error is returned first
	cfg := &config.Config{
		GitLab: config.GitLabConfig{Assignee: "", Reviewer: "valid"},
		GitHub: config.GitHubConfig{Assignee: "", Reviewer: "valid"},
	}
	err := cfg.Validate()
	if !errors.Is(err, config.ErrGitLabAssigneeEmpty) {
		t.Errorf("Expected GitLab assignee error first, got: %v", err)
	}

	// Test that GitLab reviewer error comes before GitHub errors
	cfg = &config.Config{
		GitLab: config.GitLabConfig{Assignee: "valid", Reviewer: ""},
		GitHub: config.GitHubConfig{Assignee: "", Reviewer: "valid"},
	}
	err = cfg.Validate()
	if !errors.Is(err, config.ErrGitLabReviewerEmpty) {
		t.Errorf("Expected GitLab reviewer error before GitHub errors, got: %v", err)
	}

	// Test that GitHub assignee error is returned after GitLab passes
	cfg = &config.Config{
		GitLab: config.GitLabConfig{Assignee: "valid", Reviewer: "valid"},
		GitHub: config.GitHubConfig{Assignee: "", Reviewer: "valid"},
	}
	err = cfg.Validate()
	if !errors.Is(err, config.ErrGitHubAssigneeEmpty) {
		t.Errorf("Expected GitHub assignee error, got: %v", err)
	}
}

// TestAllFieldsValid tests that valid configuration passes all validation.
func TestAllFieldsValid(t *testing.T) {
	tests := []struct {
		name   string
		config config.Config
	}{
		{
			name: "simple valid usernames",
			config: config.Config{
				GitLab: config.GitLabConfig{Assignee: "john", Reviewer: "jane"},
				GitHub: config.GitHubConfig{Assignee: "bob", Reviewer: "alice"},
			},
		},
		{
			name: "usernames with hyphens",
			config: config.Config{
				GitLab: config.GitLabConfig{Assignee: "john-doe", Reviewer: "jane-smith"},
				GitHub: config.GitHubConfig{Assignee: "bob-jones", Reviewer: "alice-wilson"},
			},
		},
		{
			name: "usernames with underscores",
			config: config.Config{
				GitLab: config.GitLabConfig{Assignee: "john_doe", Reviewer: "jane_smith"},
				GitHub: config.GitHubConfig{Assignee: "bob_jones", Reviewer: "alice_wilson"},
			},
		},
		{
			name: "usernames with numbers",
			config: config.Config{
				GitLab: config.GitLabConfig{Assignee: "user123", Reviewer: "reviewer456"},
				GitHub: config.GitHubConfig{Assignee: "dev789", Reviewer: "tester012"},
			},
		},
		{
			name: "mixed format usernames",
			config: config.Config{
				GitLab: config.GitLabConfig{Assignee: "john-doe_123", Reviewer: "jane_smith-456"},
				GitHub: config.GitHubConfig{Assignee: "bob123-test", Reviewer: "alice_review-01"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config
			err := cfg.Validate()
			if err != nil {
				t.Errorf("Expected valid config to pass validation, got error: %v", err)
			}
		})
	}
}

// ========== Load() Function Tests ==========

// TestLoad tests successful config loading scenarios.
func TestLoad(t *testing.T) {
	tests := []struct {
		name           string
		configYAML     string
		expectedConfig config.Config
	}{
		{
			name:       "valid standard config",
			configYAML: validConfigYAML,
			expectedConfig: config.Config{
				GitLab: config.GitLabConfig{Assignee: "john-doe", Reviewer: "jane-smith"},
				GitHub: config.GitHubConfig{Assignee: "bob-jones", Reviewer: "alice-wilson"},
			},
		},
		{
			name:       "config with whitespace (auto-trimmed)",
			configYAML: validConfigWithWhitespace,
			expectedConfig: config.Config{
				GitLab: config.GitLabConfig{Assignee: "john-doe", Reviewer: "jane-smith"},
				GitHub: config.GitHubConfig{Assignee: "bob-jones", Reviewer: "alice-wilson"},
			},
		},
		{
			name:       "minimal usernames (1 char)",
			configYAML: validMinimalUsernames,
			expectedConfig: config.Config{
				GitLab: config.GitLabConfig{Assignee: "a", Reviewer: "b"},
				GitHub: config.GitHubConfig{Assignee: "c", Reviewer: "d"},
			},
		},
		{
			name:       "maximum length usernames (39 chars)",
			configYAML: validMaxLengthUsernames,
			expectedConfig: config.Config{
				GitLab: config.GitLabConfig{
					Assignee: "abcdefghijklmnopqrstuvwxyz1234567890123",
					Reviewer: "zyxwvutsrqponmlkjihgfedcba9876543210987",
				},
				GitHub: config.GitHubConfig{
					Assignee: "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123",
					Reviewer: "ZYXWVUTSRQPONMLKJIHGFEDCBA9876543210987",
				},
			},
		},
		{
			name:       "config with comments",
			configYAML: validConfigWithComments,
			expectedConfig: config.Config{
				GitLab: config.GitLabConfig{Assignee: "john-doe", Reviewer: "jane-smith"},
				GitHub: config.GitHubConfig{Assignee: "bob-jones", Reviewer: "alice-wilson"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestConfig(t, tt.configYAML)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Expected Load() to succeed, got error: %v", err)
			}

			if cfg == nil {
				t.Fatal("Expected config to be non-nil")
			}

			// Verify all fields match expected values
			if cfg.GitLab.Assignee != tt.expectedConfig.GitLab.Assignee {
				t.Errorf("GitLab.Assignee: expected '%s', got '%s'",
					tt.expectedConfig.GitLab.Assignee, cfg.GitLab.Assignee)
			}
			if cfg.GitLab.Reviewer != tt.expectedConfig.GitLab.Reviewer {
				t.Errorf("GitLab.Reviewer: expected '%s', got '%s'",
					tt.expectedConfig.GitLab.Reviewer, cfg.GitLab.Reviewer)
			}
			if cfg.GitHub.Assignee != tt.expectedConfig.GitHub.Assignee {
				t.Errorf("GitHub.Assignee: expected '%s', got '%s'",
					tt.expectedConfig.GitHub.Assignee, cfg.GitHub.Assignee)
			}
			if cfg.GitHub.Reviewer != tt.expectedConfig.GitHub.Reviewer {
				t.Errorf("GitHub.Reviewer: expected '%s', got '%s'",
					tt.expectedConfig.GitHub.Reviewer, cfg.GitHub.Reviewer)
			}
		})
	}
}

// TestLoadFileNotFound tests error handling when config file doesn't exist.
func TestLoadFileNotFound(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(*testing.T)
		expectError bool
	}{
		{
			name: "config file doesn't exist",
			setupFunc: func(t *testing.T) {
				t.Helper()
				// Create temp home but no config file
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
			},
			expectError: true,
		},
		{
			name: "config directory exists but file missing",
			setupFunc: func(t *testing.T) {
				t.Helper()
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				// Create directory but no file
				configDir := filepath.Join(tmpHome, ".config", "auto-mr")
				if err := os.MkdirAll(configDir, 0o755); err != nil {
					t.Fatalf("Failed to create config directory: %v", err)
				}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupFunc(t)

			cfg, err := config.Load()

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				if !errors.Is(err, config.ErrConfigNotFound) {
					t.Errorf("Expected ErrConfigNotFound, got: %v", err)
				}
				// Verify error message includes config path
				if !strings.Contains(err.Error(), "config.yml") {
					t.Errorf("Error should mention config file path: %v", err)
				}
				if cfg != nil {
					t.Error("Expected nil config on error")
				}
			}
		})
	}
}

// TestLoadMalformedYAML tests error handling for malformed YAML files.
func TestLoadMalformedYAML(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
	}{
		{"incorrect indentation", malformedYAMLIndentation},
		{"tabs instead of spaces", malformedYAMLTabs},
		{"unclosed quotes", malformedYAMLUnclosedQuote},
		{"empty file", emptyYAML},
		{"only whitespace", whitespaceOnlyYAML},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestConfig(t, tt.configYAML)

			cfg, err := config.Load()
			if err == nil {
				t.Fatal("Expected error for malformed YAML, got nil")
			}

			// Verify error mentions parsing failure
			if !strings.Contains(err.Error(), "failed to parse config file") &&
				!strings.Contains(err.Error(), "invalid configuration") {
				t.Errorf("Error should mention parsing or validation: %v", err)
			}

			if cfg != nil {
				t.Error("Expected nil config on error")
			}
		})
	}
}

// TestLoadValidationFailures tests that Load() calls Validate() and propagates errors.
func TestLoadValidationFailures(t *testing.T) {
	tests := []struct {
		name          string
		configYAML    string
		expectedError error
	}{
		{
			name:          "missing GitLab assignee",
			configYAML:    configMissingGitLabAssignee,
			expectedError: config.ErrGitLabAssigneeEmpty,
		},
		{
			name:          "missing GitLab reviewer",
			configYAML:    configMissingGitLabReviewer,
			expectedError: config.ErrGitLabReviewerEmpty,
		},
		{
			name:          "missing GitHub assignee",
			configYAML:    configMissingGitHubAssignee,
			expectedError: config.ErrGitHubAssigneeEmpty,
		},
		{
			name:          "missing GitHub reviewer",
			configYAML:    configMissingGitHubReviewer,
			expectedError: config.ErrGitHubReviewerEmpty,
		},
		{
			name:          "invalid GitLab assignee format (starts with hyphen)",
			configYAML:    configInvalidGitLabAssigneeFormat,
			expectedError: config.ErrGitLabAssigneeInvalid,
		},
		{
			name:          "invalid GitLab reviewer format (contains period)",
			configYAML:    configInvalidGitLabReviewerFormat,
			expectedError: config.ErrGitLabReviewerInvalid,
		},
		{
			name:          "invalid GitHub assignee format (contains @)",
			configYAML:    configInvalidGitHubAssigneeFormat,
			expectedError: config.ErrGitHubAssigneeInvalid,
		},
		{
			name:          "invalid GitHub reviewer format (ends with underscore)",
			configYAML:    configInvalidGitHubReviewerFormat,
			expectedError: config.ErrGitHubReviewerInvalid,
		},
		{
			name:          "username too long (40+ chars)",
			configYAML:    configUsernameTooLong,
			expectedError: config.ErrGitLabAssigneeInvalid,
		},
		{
			name:          "unicode characters in username",
			configYAML:    configUnicodeCharacters,
			expectedError: config.ErrGitLabAssigneeInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestConfig(t, tt.configYAML)

			cfg, err := config.Load()
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}

			// Verify correct error type
			if !errors.Is(err, tt.expectedError) {
				t.Errorf("Expected error %v, got: %v", tt.expectedError, err)
			}

			// Verify error wrapping mentions validation
			if !strings.Contains(err.Error(), "invalid configuration") {
				t.Errorf("Error should be wrapped with 'invalid configuration': %v", err)
			}

			if cfg != nil {
				t.Error("Expected nil config on validation error")
			}
		})
	}
}

// TestLoadIntegration tests the complete Load() → Validate() workflow.
func TestLoadIntegration(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		wantError  bool
		errorType  error
	}{
		{
			name:       "complete successful workflow",
			configYAML: validConfigYAML,
			wantError:  false,
		},
		{
			name:       "whitespace trimming integration",
			configYAML: validConfigWithWhitespace,
			wantError:  false,
		},
		{
			name:       "file not found → error",
			configYAML: "", // Don't create file
			wantError:  true,
			errorType:  config.ErrConfigNotFound,
		},
		{
			name:       "malformed YAML → error",
			configYAML: malformedYAMLUnclosedQuote,
			wantError:  true,
		},
		{
			name:       "validation failure → error",
			configYAML: configMissingGitLabAssignee,
			wantError:  true,
			errorType:  config.ErrGitLabAssigneeEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.configYAML != "" {
				setupTestConfig(t, tt.configYAML)
			} else {
				// For file not found test
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
			}

			cfg, err := config.Load()

			if tt.wantError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				if tt.errorType != nil && !errors.Is(err, tt.errorType) {
					t.Errorf("Expected error type %v, got: %v", tt.errorType, err)
				}
				if cfg != nil {
					t.Error("Expected nil config on error")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected success, got error: %v", err)
				}
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}

				// Verify idempotency: can call Validate() again
				if err := cfg.Validate(); err != nil {
					t.Errorf("Second Validate() call should succeed: %v", err)
				}
			}
		})
	}
}

// TestLoadEdgeCases tests boundary conditions and unusual scenarios.
func TestLoadEdgeCases(t *testing.T) {
	t.Run("config with extra YAML fields", func(t *testing.T) {
		extraFieldsYAML := `
gitlab:
  assignee: john-doe
  reviewer: jane-smith
  extra_field: ignored
github:
  assignee: bob-jones
  reviewer: alice-wilson
  another_field: also_ignored
unknown_section:
  foo: bar
`
		setupTestConfig(t, extraFieldsYAML)

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("Load should ignore extra fields, got error: %v", err)
		}
		if cfg == nil {
			t.Fatal("Expected non-nil config")
		}
	})

	t.Run("config with YAML anchors and aliases", func(t *testing.T) {
		yamlWithAnchors := `
gitlab:
  assignee: &default_user john-doe
  reviewer: *default_user
github:
  assignee: bob-jones
  reviewer: alice-wilson
`
		setupTestConfig(t, yamlWithAnchors)

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("Load should handle YAML anchors, got error: %v", err)
		}
		if cfg.GitLab.Assignee != "john-doe" || cfg.GitLab.Reviewer != "john-doe" {
			t.Error("YAML anchor/alias not resolved correctly")
		}
	})

	t.Run("verify Load respects $HOME environment variable", func(t *testing.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		// Load should fail since no config exists
		_, err := config.Load()
		if err == nil {
			t.Fatal("Expected error for missing config")
		}
		if !errors.Is(err, config.ErrConfigNotFound) {
			t.Errorf("Expected ErrConfigNotFound, got: %v", err)
		}

		// Verify error message includes the temp home path
		if !strings.Contains(err.Error(), tmpHome) {
			t.Errorf("Error should include temp home path %s: %v", tmpHome, err)
		}
	})

	t.Run("config with only GitLab section (GitHub validation fails)", func(t *testing.T) {
		onlyGitLabYAML := `
gitlab:
  assignee: john-doe
  reviewer: jane-smith
`
		setupTestConfig(t, onlyGitLabYAML)

		cfg, err := config.Load()
		if err == nil {
			t.Fatal("Expected validation error for missing GitHub section")
		}
		if !errors.Is(err, config.ErrGitHubAssigneeEmpty) {
			t.Errorf("Expected ErrGitHubAssigneeEmpty, got: %v", err)
		}
		if cfg != nil {
			t.Error("Expected nil config on validation error")
		}
	})

	t.Run("config with only GitHub section (GitLab validation fails)", func(t *testing.T) {
		onlyGitHubYAML := `
github:
  assignee: bob-jones
  reviewer: alice-wilson
`
		setupTestConfig(t, onlyGitHubYAML)

		cfg, err := config.Load()
		if err == nil {
			t.Fatal("Expected validation error for missing GitLab section")
		}
		if !errors.Is(err, config.ErrGitLabAssigneeEmpty) {
			t.Errorf("Expected ErrGitLabAssigneeEmpty, got: %v", err)
		}
		if cfg != nil {
			t.Error("Expected nil config on validation error")
		}
	})
}
