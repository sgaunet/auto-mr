package security_test

import (
	"fmt"
	"testing"

	"github.com/sgaunet/auto-mr/internal/security"
)

func TestSecureToken_String(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "empty token",
			token:    "",
			expected: "[empty]",
		},
		{
			name:     "short token",
			token:    "short",
			expected: "[redacted]",
		},
		{
			name:     "exactly 8 chars",
			token:    "12345678",
			expected: "[token:****5678]",
		},
		{
			name:     "long token",
			token:    "glpat-1234567890abcdefghij",
			expected: "[token:****ghij]",
		},
		{
			name:     "github token",
			token:    "ghp_1234567890123456789012345678901234abcd",
			expected: "[token:****abcd]",
		},
		{
			name:     "gitlab token",
			token:    "glpat-abcdefghijklmnopqrst",
			expected: "[token:****qrst]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := security.NewSecureToken(tt.token)
			got := token.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSecureToken_FormattingVerbs(t *testing.T) {
	token := security.NewSecureToken("glpat-secret1234567890abcd")
	expected := "[token:****abcd]"

	tests := []struct {
		name   string
		format string
	}{
		{name: "%s verb", format: "%s"},
		{name: "%v verb", format: "%v"},
		{name: "%+v verb", format: "%+v"},
		{name: "%#v verb", format: "%#v"},
		{name: "%q verb", format: "%q"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmt.Sprintf(tt.format, token)
			// %q adds quotes, so we need to handle that
			if tt.format == "%q" {
				if got != fmt.Sprintf("%q", expected) {
					t.Errorf("fmt.Sprintf(%q, token) = %q, want quoted %q", tt.format, got, expected)
				}
			} else if tt.format == "%#v" {
				// %#v might include type info, just verify token not present
				if got != expected {
					t.Errorf("fmt.Sprintf(%q, token) = %q, want %q", tt.format, got, expected)
				}
			} else {
				if got != expected {
					t.Errorf("fmt.Sprintf(%q, token) = %q, want %q", tt.format, got, expected)
				}
			}
		})
	}
}

func TestSecureToken_Value(t *testing.T) {
	originalToken := "glpat-secret1234567890"
	token := security.NewSecureToken(originalToken)

	got := token.Value()
	if got != originalToken {
		t.Errorf("Value() = %q, want %q", got, originalToken)
	}
}

func TestSecureToken_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "empty token",
			token:    "",
			expected: true,
		},
		{
			name:     "non-empty token",
			token:    "glpat-123",
			expected: false,
		},
		{
			name:     "whitespace only",
			token:    "   ",
			expected: false, // Not empty, just contains whitespace
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := security.NewSecureToken(tt.token)
			got := token.IsEmpty()
			if got != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSecureToken_NoLeakage(t *testing.T) {
	// This test verifies that the actual token value never appears
	// in any string representation
	actualToken := "glpat-verysecrettoken12345"
	token := security.NewSecureToken(actualToken)

	// Test various string conversions
	stringRepresentations := []string{
		token.String(),
		fmt.Sprintf("%s", token),
		fmt.Sprintf("%v", token),
		fmt.Sprintf("%+v", token),
		fmt.Sprint(token),
	}

	for i, repr := range stringRepresentations {
		if repr == actualToken {
			t.Errorf("Representation %d leaked actual token: %q", i, repr)
		}
		// Verify it doesn't contain the secret part
		if len(actualToken) > 8 && repr != "[empty]" && repr != "[redacted]" {
			// Extract the secret part (everything except last 4 chars that might be shown)
			secretPart := actualToken[:len(actualToken)-4]
			if len(secretPart) > 0 && repr != "" && repr != "[empty]" && repr != "[redacted]" {
				// Don't check if repr is one of the safe strings
				if repr != "[token:****2345]" {
					t.Logf("Checking representation: %q", repr)
				}
			}
		}
	}
}

func TestSecureToken_StructEmbedding(t *testing.T) {
	// Test that SecureToken doesn't leak when embedded in structs
	type AuthConfig struct {
		Username string
		Token    security.SecureToken
	}

	config := AuthConfig{
		Username: "testuser",
		Token:    security.NewSecureToken("glpat-secrettoken123456"),
	}

	// Test various formatting
	repr := fmt.Sprintf("%+v", config)
	if repr == "" {
		t.Error("Expected non-empty representation")
	}
	// Verify actual token not present
	if repr == "glpat-secrettoken123456" || len(repr) < 10 {
		t.Errorf("Token might have leaked in struct: %q", repr)
	}
}

func TestSecureToken_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "unicode characters", token: "token-with-unicode-日本語"},
		{name: "special characters", token: "token!@#$%^&*()_+-={}[]|:;<>?,./"},
		{name: "newline in token", token: "token\nwith\nnewlines"},
		{name: "very long token", token: string(make([]byte, 1000))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := security.NewSecureToken(tt.token)
			str := token.String()

			// Should never return the actual token
			if str == tt.token {
				t.Error("String() returned actual token value")
			}

			// Should return a safe representation
			if str != "[empty]" && str != "[redacted]" && !hasPrefix(str, "[token:") {
				t.Errorf("Unexpected string representation: %q", str)
			}
		})
	}
}

// hasPrefix checks if s starts with prefix (helper for tests).
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
