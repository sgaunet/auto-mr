package security_test

import (
	"fmt"
	"testing"

	"github.com/sgaunet/auto-mr/internal/security"
)

// TestEndToEnd_TokenLeakagePrevention verifies that tokens cannot leak
// through various real-world scenarios.
func TestEndToEnd_TokenLeakagePrevention(t *testing.T) {
	// Simulate a real GitLab token
	actualToken := "glpat-abcdefghijklmnopqrst1234567890"
	secureToken := security.NewSecureToken(actualToken)

	t.Run("struct with token field", func(t *testing.T) {
		type AuthConfig struct {
			URL   string
			Token security.SecureToken
			User  string
		}

		config := AuthConfig{
			URL:   "https://gitlab.com",
			Token: secureToken,
			User:  "testuser",
		}

		// Try to leak through various fmt operations
		outputs := []string{
			fmt.Sprintf("%v", config),
			fmt.Sprintf("%+v", config),
			fmt.Sprintf("%#v", config),
			fmt.Sprint(config),
		}

		for i, output := range outputs {
			if containsSubstring(output, actualToken) {
				t.Errorf("Output %d leaked actual token", i)
			}
		}
	})

	t.Run("error wrapping", func(t *testing.T) {
		// Simulate an error that accidentally includes a token
		baseErr := fmt.Errorf("authentication failed with token: %s", actualToken)
		sanitizedErr := security.SanitizeError(baseErr)

		if containsSubstring(sanitizedErr.Error(), actualToken) {
			t.Error("Sanitized error still contains actual token")
		}

		if !containsSubstring(sanitizedErr.Error(), "[gitlab-token-redacted]") {
			t.Error("Sanitized error should contain redaction marker")
		}
	})

	t.Run("map with token", func(t *testing.T) {
		data := map[string]any{
			"token":    actualToken,
			"endpoint": "https://gitlab.com/api/v4",
		}

		sanitized := security.SanitizeMap(data)

		if tokenVal, ok := sanitized["token"].(string); ok {
			if tokenVal == actualToken {
				t.Error("Map sanitization failed to redact token")
			}
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		// Verify thread safety
		done := make(chan bool)
		const goroutines = 100

		for range goroutines {
			go func() {
				_ = secureToken.String()
				_ = security.SanitizeString(actualToken)
				_ = security.SanitizeMap(map[string]any{"token": actualToken})
				done <- true
			}()
		}

		for range goroutines {
			<-done
		}
		// If we get here without race conditions, test passes
	})
}

// TestRealWorldScenarios tests scenarios that might occur in production.
func TestRealWorldScenarios(t *testing.T) {
	t.Run("git push error with token in url", func(t *testing.T) {
		// Simulate an error message from git that includes credentials
		errorMsg := "failed to push to https://oauth2:glpat-secret123@gitlab.com/repo.git: permission denied"
		sanitized := security.SanitizeString(errorMsg)

		if containsSubstring(sanitized, "glpat-") {
			t.Error("Failed to sanitize token from git error")
		}
	})

	t.Run("http basic auth logging", func(t *testing.T) {
		// Simulate logging auth details
		token := security.NewSecureToken("ghp_1234567890123456789012345678901234abcd")

		logMessage := fmt.Sprintf("Authenticating with token: %s", token)

		if containsSubstring(logMessage, "ghp_") {
			t.Error("Token leaked in log message")
		}

		if !containsSubstring(logMessage, "[token:") {
			t.Error("Expected masked token format in log message")
		}
	})

	t.Run("ssh key path exposure", func(t *testing.T) {
		// Test that full paths are masked
		fullPath := "/Users/sensitive-username/.ssh/id_ed25519"
		masked := security.MaskSSHKeyPath(fullPath)

		if containsSubstring(masked, "sensitive-username") {
			t.Errorf("SSH path masking leaked username: %s", masked)
		}

		if !containsSubstring(masked, "~/.ssh/") {
			t.Errorf("Expected tilde notation in masked path: %s", masked)
		}
	})

	t.Run("multiple tokens in same string", func(t *testing.T) {
		// Test that all tokens are sanitized
		input := "GitLab: glpat-token1 and GitHub: ghp_token2token2token2token2token2token2token2"
		sanitized := security.SanitizeString(input)

		if containsSubstring(sanitized, "glpat-") {
			t.Error("Failed to sanitize GitLab token")
		}

		if containsSubstring(sanitized, "ghp_") {
			t.Error("Failed to sanitize GitHub token")
		}
	})
}

// TestSecurityRegression ensures known vulnerabilities stay fixed.
func TestSecurityRegression(t *testing.T) {
	t.Run("issue_46_token_in_debug_log", func(t *testing.T) {
		// Original issue: tokens could leak in debug mode
		token := security.NewSecureToken("glpat-originalsecret123456")

		// Simulate debug logging
		debugMsg := fmt.Sprintf("Debug: using token %v", token)

		if containsSubstring(debugMsg, "originalsecret") {
			t.Error("Regression: token leaked in debug log (issue #46)")
		}
	})

	t.Run("struct_stringification", func(t *testing.T) {
		// Ensure structs containing SecureToken don't leak
		type Config struct {
			Token security.SecureToken
		}

		cfg := Config{Token: security.NewSecureToken("glpat-structsecret12345")}
		str := fmt.Sprintf("%+v", cfg)

		if containsSubstring(str, "structsecret") {
			t.Error("Token leaked through struct stringification")
		}
	})
}

// TestSanitizationCompleteness verifies all sanitization patterns work together.
func TestSanitizationCompleteness(t *testing.T) {
	// Test data with various token formats
	testCases := []struct {
		name     string
		input    string
		mustNot  []string // Strings that must NOT appear in output
		mustHave []string // Strings that MUST appear in output
	}{
		{
			name:     "gitlab token in url",
			input:    "Pushing to https://oauth2:glpat-abc123@gitlab.com/repo.git",
			mustNot:  []string{"glpat-abc123"},
			mustHave: []string{"[gitlab-token-redacted]"},
		},
		{
			name:     "github token in auth header",
			input:    "Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz1234567890",
			mustNot:  []string{"ghp_abcdefghijklmnopqrstuvwxyz1234567890"},
			mustHave: []string{"[github-token-redacted]"},
		},
		{
			name:     "mixed tokens",
			input:    "Using glpat-123456 for GitLab and ghp_456456456456456456456456456456456456 for GitHub",
			mustNot:  []string{"glpat-123456", "ghp_456"},
			mustHave: []string{"[gitlab-token-redacted]", "[github-token-redacted]"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := security.SanitizeString(tc.input)

			for _, forbidden := range tc.mustNot {
				if containsSubstring(output, forbidden) {
					t.Errorf("Output contains forbidden string %q: %s", forbidden, output)
				}
			}

			for _, required := range tc.mustHave {
				if !containsSubstring(output, required) {
					t.Errorf("Output missing required string %q: %s", required, output)
				}
			}
		})
	}
}

// containsSubstring is a helper that checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

// findSubstring checks if substr exists in s using simple search.
func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
