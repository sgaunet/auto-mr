package security

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	// minSSHPathParts is the minimum number of parts needed for SSH path masking.
	minSSHPathParts = 2
)

var (
	// Token regex patterns compiled once using sync.Once for performance.
	gitlabTokenRegex *regexp.Regexp
	githubTokenRegex *regexp.Regexp
	bearerTokenRegex *regexp.Regexp
	authHeaderRegex  *regexp.Regexp
	regexOnce        sync.Once

	// errSanitized is the error type for sanitized errors.
	errSanitized = errors.New("sanitized error")
)

// compileRegexPatterns initializes all regex patterns once.
func compileRegexPatterns() {
	regexOnce.Do(func() {
		// GitLab personal access tokens: glpat-[6+ chars]
		// Real tokens are 20+ chars, but we catch shorter ones for safety
		gitlabTokenRegex = regexp.MustCompile(`glpat-[a-zA-Z0-9_-]{6,}`)

		// GitHub personal access tokens: ghp_/gho_/ghs_ + 20+ chars
		// Real tokens are 36+ chars, but we catch shorter ones for safety
		githubTokenRegex = regexp.MustCompile(`gh[ops]_[a-zA-Z0-9]{20,}`)

		// Generic bearer tokens: long base64-like strings (40-200 chars)
		bearerTokenRegex = regexp.MustCompile(`\b[A-Za-z0-9+/=]{40,200}\b`)

		// Authorization headers: "Authorization: Bearer <token>" or "Authorization: <token>"
		// Captures both Basic auth (base64) and Bearer tokens
		authHeaderRegex = regexp.MustCompile(`(?i)authorization:\s*(?:bearer|basic)\s+[a-zA-Z0-9+/=_-]{10,}`)
	})
}

// SanitizeString removes sensitive tokens from a string using compiled regex patterns.
// It detects and redacts GitLab tokens (glpat-*), GitHub tokens (ghp_/gho_/ghs_*),
// authorization headers, and generic bearer tokens.
// This provides defense-in-depth protection against token leakage.
//
// Thread Safety: Safe for concurrent use after first call (regex patterns compiled via sync.Once).
func SanitizeString(s string) string {
	compileRegexPatterns()

	// Replace GitLab tokens
	s = gitlabTokenRegex.ReplaceAllString(s, "[gitlab-token-redacted]")

	// Replace GitHub tokens
	s = githubTokenRegex.ReplaceAllString(s, "[github-token-redacted]")

	// Replace authorization headers
	s = authHeaderRegex.ReplaceAllString(s, "Authorization: [redacted]")

	// Replace generic bearer tokens (do this last to avoid over-redaction)
	// Only redact if not already redacted by previous patterns
	if strings.Contains(s, "glpat-") || strings.Contains(s, "ghp_") ||
		strings.Contains(s, "gho_") || strings.Contains(s, "ghs_") {
		return s
	}
	s = bearerTokenRegex.ReplaceAllString(s, "[token-redacted]")

	return s
}

// SanitizeError wraps an error with [SanitizeString] applied to its message.
// Returns nil if err is nil. The original error chain is not preserved;
// the returned error wraps an internal errSanitized sentinel.
func SanitizeError(err error) error {
	if err == nil {
		return nil
	}
	sanitized := SanitizeString(err.Error())
	return fmt.Errorf("%w: %s", errSanitized, sanitized)
}

// MaskSSHKeyPath obfuscates SSH key file paths for safe logging.
// Converts absolute paths to relative paths from home directory.
//
// Example:
//
//	/Users/john/.ssh/id_ed25519 -> ~/.ssh/id_ed25519
//	/home/jane/.ssh/id_rsa -> ~/.ssh/id_rsa
func MaskSSHKeyPath(path string) string {
	if path == "" {
		return ""
	}

	// Replace home directory with tilde
	if strings.Contains(path, "/.ssh/") {
		// Extract just the filename and .ssh directory
		parts := strings.Split(path, "/.ssh/")
		if len(parts) >= minSSHPathParts {
			return "~/.ssh/" + filepath.Base(parts[len(parts)-1])
		}
	}

	// Fallback: just show the filename if path doesn't match expected pattern
	return filepath.Base(path)
}

// SanitizeMap redacts values whose keys match common sensitive names
// (token, password, secret, api_key, auth, credential, authorization).
// Non-sensitive string values are also passed through [SanitizeString].
// Returns nil if m is nil.
func SanitizeMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	sensitiveKeys := []string{
		"token", "password", "secret", "api_key", "apikey",
		"auth", "credential", "authorization",
	}

	result := make(map[string]any, len(m))
	for k, v := range m {
		// Check if key name suggests sensitive data
		lowerKey := strings.ToLower(k)
		isSensitive := false
		for _, sensitiveKey := range sensitiveKeys {
			if strings.Contains(lowerKey, sensitiveKey) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			// Redact sensitive values
			result[k] = maskRedacted
		} else {
			// Keep non-sensitive values, but recursively sanitize strings
			if str, ok := v.(string); ok {
				result[k] = SanitizeString(str)
			} else {
				result[k] = v
			}
		}
	}

	return result
}
