package config_test

import (
	"errors"
	"testing"

	"github.com/sgaunet/auto-mr/pkg/config"
)

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
