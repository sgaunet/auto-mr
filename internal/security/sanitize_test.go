package security_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sgaunet/auto-mr/internal/security"
)

func TestSanitizeString_GitLabTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "gitlab token",
			input:    "Using token: glpat-1234567890abcdefghij",
			expected: "Using token: [gitlab-token-redacted]",
		},
		{
			name:     "multiple gitlab tokens",
			input:    "Token1: glpat-aaaaaaaaaaaaaaaaaaaa Token2: glpat-bbbbbbbbbbbbbbbbbbbb",
			expected: "Token1: [gitlab-token-redacted] Token2: [gitlab-token-redacted]",
		},
		{
			name:     "gitlab token in url",
			input:    "https://oauth2:glpat-secret@gitlab.com/repo.git",
			expected: "https://oauth2:[gitlab-token-redacted]@gitlab.com/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := security.SanitizeString(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSanitizeString_GitHubTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "github personal token",
			input:    "Token: ghp_1234567890123456789012345678901234abcd",
			expected: "Token: [github-token-redacted]",
		},
		{
			name:     "github oauth token",
			input:    "Token: gho_1234567890123456789012345678901234abcd",
			expected: "Token: [github-token-redacted]",
		},
		{
			name:     "github server token",
			input:    "Token: ghs_1234567890123456789012345678901234abcd",
			expected: "Token: [github-token-redacted]",
		},
		{
			name:     "github token in url",
			input:    "https://x-access-token:ghp_123456789012345678901234567890123456@github.com/repo.git",
			expected: "https://x-access-token:[github-token-redacted]@github.com/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := security.SanitizeString(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSanitizeString_AuthorizationHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bearer token",
			input:    "Authorization: Bearer abc123def456ghi789jkl012mno345pqr678",
			expected: "Authorization: [redacted]",
		},
		{
			name:     "basic auth",
			input:    "Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ=",
			expected: "Authorization: [redacted]",
		},
		{
			name:     "case insensitive",
			input:    "authorization: bearer ABC123DEF456GHI789JKL012MNO345PQR678",
			expected: "Authorization: [redacted]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := security.SanitizeString(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSanitizeString_NoTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "normal text",
			input: "This is a normal log message without any tokens",
		},
		{
			name:  "short strings",
			input: "short",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "url without token",
			input: "https://gitlab.com/user/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := security.SanitizeString(tt.input)
			if got != tt.input {
				t.Errorf("SanitizeString() modified input without tokens: got %q, want %q", got, tt.input)
			}
		})
	}
}

func TestSanitizeError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		shouldExist bool
		shouldNot   string
	}{
		{
			name:        "nil error",
			err:         nil,
			shouldExist: false,
		},
		{
			name:        "error with gitlab token",
			err:         errors.New("failed to push: glpat-1234567890abcdefghij"),
			shouldExist: true,
			shouldNot:   "glpat-",
		},
		{
			name:        "error with github token",
			err:         errors.New("auth failed: ghp_1234567890123456789012345678901234abcd"),
			shouldExist: true,
			shouldNot:   "ghp_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := security.SanitizeError(tt.err)

			if !tt.shouldExist {
				if got != nil {
					t.Errorf("SanitizeError() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Error("SanitizeError() returned nil, want error")
				return
			}

			errMsg := got.Error()
			if tt.shouldNot != "" && strings.Contains(errMsg, tt.shouldNot) {
				t.Errorf("SanitizeError() still contains %q: %q", tt.shouldNot, errMsg)
			}
		})
	}
}

func TestMaskSSHKeyPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "standard ed25519 key",
			path:     "/Users/john/.ssh/id_ed25519",
			expected: "~/.ssh/id_ed25519",
		},
		{
			name:     "standard rsa key",
			path:     "/home/jane/.ssh/id_rsa",
			expected: "~/.ssh/id_rsa",
		},
		{
			name:     "custom key name",
			path:     "/Users/bob/.ssh/my_custom_key",
			expected: "~/.ssh/my_custom_key",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "path without .ssh",
			path:     "/Users/alice/keys/mykey",
			expected: "mykey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := security.MaskSSHKeyPath(tt.path)
			if got != tt.expected {
				t.Errorf("MaskSSHKeyPath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSanitizeMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "nil map",
			input:    nil,
			expected: nil,
		},
		{
			name: "redact token key",
			input: map[string]any{
				"token":    "glpat-secret123",
				"username": "testuser",
			},
			expected: map[string]any{
				"token":    "[redacted]",
				"username": "testuser",
			},
		},
		{
			name: "redact password key",
			input: map[string]any{
				"password": "secret123",
				"email":    "test@example.com",
			},
			expected: map[string]any{
				"password": "[redacted]",
				"email":    "test@example.com",
			},
		},
		{
			name: "redact api_key",
			input: map[string]any{
				"api_key": "key123",
				"name":    "test",
			},
			expected: map[string]any{
				"api_key": "[redacted]",
				"name":    "test",
			},
		},
		{
			name: "case insensitive matching",
			input: map[string]any{
				"Token":        "secret1",
				"PASSWORD":     "secret2",
				"Authorization": "secret3",
			},
			expected: map[string]any{
				"Token":        "[redacted]",
				"PASSWORD":     "[redacted]",
				"Authorization": "[redacted]",
			},
		},
		{
			name: "sanitize token in non-sensitive value",
			input: map[string]any{
				"url": "https://gitlab.com?token=glpat-123456789012345678901234",
			},
			expected: map[string]any{
				"url": "https://gitlab.com?token=[gitlab-token-redacted]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := security.SanitizeMap(tt.input)

			if tt.expected == nil {
				if got != nil {
					t.Errorf("SanitizeMap() = %v, want nil", got)
				}
				return
			}

			if len(got) != len(tt.expected) {
				t.Errorf("SanitizeMap() returned map with %d entries, want %d", len(got), len(tt.expected))
			}

			for key, expectedVal := range tt.expected {
				gotVal, exists := got[key]
				if !exists {
					t.Errorf("SanitizeMap() missing key %q", key)
					continue
				}
				if gotVal != expectedVal {
					t.Errorf("SanitizeMap()[%q] = %v, want %v", key, gotVal, expectedVal)
				}
			}
		})
	}
}

func TestSanitizeString_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "very long string",
			input: strings.Repeat("a", 10000) + "glpat-1234567890abcdefghij" + strings.Repeat("b", 10000),
		},
		{
			name:  "multiple token types",
			input: "gitlab: glpat-12345678901234567890 github: ghp_1234567890123456789012345678901234abcd",
		},
		{
			name:  "unicode with token",
			input: "Token 日本語: glpat-1234567890abcdefghij",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := security.SanitizeString(tt.input)

			// Verify no actual tokens remain
			if strings.Contains(got, "glpat-") {
				t.Error("Sanitized string still contains gitlab token prefix")
			}
			if strings.Contains(got, "ghp_") {
				t.Error("Sanitized string still contains github token prefix")
			}
		})
	}
}
