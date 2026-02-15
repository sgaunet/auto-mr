// Package security provides token security and credential sanitization utilities.
package security

import "fmt"

const (
	// Minimum token length to show partial masking (show last 4 chars).
	minTokenLengthForPartialMask = 8
	// Number of characters to show when masking.
	maskShowChars = 4
	// maskEmpty is returned for empty tokens.
	maskEmpty = "[empty]"
	// maskRedacted is returned for short tokens.
	maskRedacted = "[redacted]"
)

// SecureToken wraps sensitive tokens to prevent accidental logging.
// The String() method returns a masked value, making it safe to use in logs,
// error messages, and fmt operations.
//
// Example:
//
//	token := NewSecureToken("glpat-secret123456")
//	fmt.Printf("Token: %s", token)  // Output: "Token: [token:****3456]"
//	fmt.Printf("Token: %v", token)  // Output: "Token: [token:****3456]"
//	fmt.Printf("Token: %+v", token) // Output: "Token: [token:****3456]"
type SecureToken struct {
	value string
}

// NewSecureToken creates a new SecureToken from a string value.
func NewSecureToken(token string) SecureToken {
	return SecureToken{value: token}
}

// String implements fmt.Stringer and returns a masked representation.
// This ensures tokens cannot leak through string formatting operations.
func (t SecureToken) String() string {
	if t.value == "" {
		return maskEmpty
	}

	// For short tokens, fully redact
	if len(t.value) < minTokenLengthForPartialMask {
		return maskRedacted
	}

	// Show last 4 characters for debugging
	lastChars := t.value[len(t.value)-maskShowChars:]
	return fmt.Sprintf("[token:****%s]", lastChars)
}

// Value returns the actual token value.
// WARNING: Use with caution. Only call this when you need the real token
// for authentication purposes. Never log or print the result.
func (t SecureToken) Value() string {
	return t.value
}

// IsEmpty returns true if the token is empty.
func (t SecureToken) IsEmpty() bool {
	return t.value == ""
}

// GoString implements fmt.GoStringer to prevent leaking in %#v formatting.
func (t SecureToken) GoString() string {
	return t.String()
}
