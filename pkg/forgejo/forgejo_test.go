// Package forgejo_test provides black box tests for the forgejo package.
package forgejo_test

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"code.gitea.io/sdk/gitea"
	"github.com/sgaunet/auto-mr/pkg/forgejo"
)

// TestNewClientMissingToken verifies that NewClient returns ErrTokenRequired when
// FORGEJO_TOKEN is not set.
func TestNewClientMissingToken(t *testing.T) {
	original := os.Getenv("FORGEJO_TOKEN")
	if err := os.Unsetenv("FORGEJO_TOKEN"); err != nil {
		t.Fatalf("failed to unset FORGEJO_TOKEN: %v", err)
	}

	defer func() {
		if original != "" {
			if err := os.Setenv("FORGEJO_TOKEN", original); err != nil {
				t.Errorf("failed to restore FORGEJO_TOKEN: %v", err)
			}
		}
	}()

	_, err := forgejo.NewClient("https://forgejo.example.com")
	if err == nil {
		t.Fatal("expected error when FORGEJO_TOKEN is not set, got nil")
	}

	if !errors.Is(err, forgejo.ErrTokenRequired) {
		t.Errorf("expected ErrTokenRequired, got: %v", err)
	}
}

// TestNewClientWhitespaceTokenTrimmed verifies that a whitespace-only FORGEJO_TOKEN
// is trimmed to empty and reported as missing, rather than producing an invalid
// Authorization header. This guards against the gitea SDK rejecting a token with a
// trailing newline ("net/http: invalid header field value for Authorization").
func TestNewClientWhitespaceTokenTrimmed(t *testing.T) {
	original := os.Getenv("FORGEJO_TOKEN")
	if err := os.Setenv("FORGEJO_TOKEN", "   \n\t "); err != nil {
		t.Fatalf("failed to set FORGEJO_TOKEN: %v", err)
	}

	defer func() {
		if original == "" {
			if err := os.Unsetenv("FORGEJO_TOKEN"); err != nil {
				t.Errorf("failed to unset FORGEJO_TOKEN: %v", err)
			}
			return
		}
		if err := os.Setenv("FORGEJO_TOKEN", original); err != nil {
			t.Errorf("failed to restore FORGEJO_TOKEN: %v", err)
		}
	}()

	_, err := forgejo.NewClient("https://forgejo.example.com")
	if !errors.Is(err, forgejo.ErrTokenRequired) {
		t.Errorf("expected ErrTokenRequired for whitespace-only token, got: %v", err)
	}
}

// TestNewClientWithToken verifies that NewClient does not return ErrTokenRequired
// when FORGEJO_TOKEN is set. The SDK performs a live version check on the base URL,
// so this test skips when the example host is unreachable.
func TestNewClientWithToken(t *testing.T) {
	if err := os.Setenv("FORGEJO_TOKEN", "test-token"); err != nil {
		t.Fatalf("failed to set FORGEJO_TOKEN: %v", err)
	}

	defer func() {
		if err := os.Unsetenv("FORGEJO_TOKEN"); err != nil {
			t.Errorf("failed to unset FORGEJO_TOKEN: %v", err)
		}
	}()

	_, err := forgejo.NewClient("https://forgejo.example.com")
	if err == nil {
		// Connected to a live server — client is valid.
		return
	}

	// If the error is the token-required sentinel, the test fails.
	if errors.Is(err, forgejo.ErrTokenRequired) {
		t.Errorf("unexpected ErrTokenRequired when token is set: %v", err)
	}
	// Any other error is a network / server error, which is expected in CI.
	t.Skipf("skipping: Forgejo server unreachable: %v", err)
}

// TestErrorSentinels verifies that all exported error sentinel values have the
// correct message text and behave correctly with errors.Is.
func TestErrorSentinels(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "ErrTokenRequired",
			err:     forgejo.ErrTokenRequired,
			wantMsg: "FORGEJO_TOKEN environment variable is required",
		},
		{
			name:    "ErrInvalidURLFormat",
			err:     forgejo.ErrInvalidURLFormat,
			wantMsg: "invalid Forgejo URL format",
		},
		{
			name:    "ErrWorkflowTimeout",
			err:     forgejo.ErrWorkflowTimeout,
			wantMsg: "timeout waiting for pipeline completion",
		},
		{
			name:    "ErrPRNotFound",
			err:     forgejo.ErrPRNotFound,
			wantMsg: "no pull request found for branch",
		},
		{
			name:    "ErrPRAlreadyExists",
			err:     forgejo.ErrPRAlreadyExists,
			wantMsg: "pull request already exists for this branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.wantMsg {
				t.Errorf("expected message %q, got %q", tt.wantMsg, tt.err.Error())
			}

			// Wrapped errors must still be detectable.
			wrapped := fmt.Errorf("wrapped: %w", tt.err)
			if !errors.Is(wrapped, tt.err) {
				t.Errorf("errors.Is failed for wrapped %s", tt.name)
			}
		})
	}
}

// TestLabelType verifies the Label type's exported field.
func TestLabelType(t *testing.T) {
	label := forgejo.Label{Name: "bug"}
	if label.Name != "bug" {
		t.Errorf("expected Name=bug, got %q", label.Name)
	}
}

// TestAPIClientInterface verifies that a nil *Client pointer satisfies APIClient at compile
// time (the var _ check in interfaces.go already guards this, but an explicit cast here
// gives a clear test failure message if the interface is broken).
func TestAPIClientInterface(t *testing.T) {
	var _ forgejo.APIClient = (*forgejo.Client)(nil)
}

// TestPRAlreadyExistsWrapping verifies that wrapping ErrPRAlreadyExists with branch
// context still allows errors.Is detection.
func TestPRAlreadyExistsWrapping(t *testing.T) {
	wrapped := fmt.Errorf("%w: head=feature, base=main: 409 conflict",
		forgejo.ErrPRAlreadyExists)

	if !errors.Is(wrapped, forgejo.ErrPRAlreadyExists) {
		t.Errorf("errors.Is failed: expected ErrPRAlreadyExists in chain, got: %v", wrapped)
	}

	if wrapped.Error() == "" {
		t.Error("wrapped error message must not be empty")
	}
}

// TestPRNotFoundWrapping verifies that wrapping ErrPRNotFound with a branch name still
// allows errors.Is detection.
func TestPRNotFoundWrapping(t *testing.T) {
	wrapped := fmt.Errorf("%w: feature-branch", forgejo.ErrPRNotFound)

	if !errors.Is(wrapped, forgejo.ErrPRNotFound) {
		t.Errorf("errors.Is failed: expected ErrPRNotFound in chain, got: %v", wrapped)
	}
}

// --- helpers testing package-level exported logic ---

// TestAggregateResultViaStatusConstants verifies that the gitea StatusState constants
// used by WaitForPipeline have the expected string values (regression guard if SDK updates).
func TestStatusStateConstants(t *testing.T) {
	tests := []struct {
		state gitea.StatusState
		want  string
	}{
		{gitea.StatusSuccess, "success"},
		{gitea.StatusPending, "pending"},
		{gitea.StatusFailure, "failure"},
		{gitea.StatusError, "error"},
		{gitea.StatusWarning, "warning"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if string(tt.state) != tt.want {
				t.Errorf("expected %q, got %q", tt.want, string(tt.state))
			}
		})
	}
}

// TestNewClientEmptyBaseURL verifies that NewClient with a whitespace-only baseURL
// still returns ErrTokenRequired when the token is absent, rather than panicking.
// (URL validation occurs after token validation in the current implementation.)
func TestNewClientEmptyBaseURL(t *testing.T) {
	original := os.Getenv("FORGEJO_TOKEN")
	if err := os.Unsetenv("FORGEJO_TOKEN"); err != nil {
		t.Fatalf("failed to unset FORGEJO_TOKEN: %v", err)
	}
	defer func() {
		if original != "" {
			if err := os.Setenv("FORGEJO_TOKEN", original); err != nil {
				t.Errorf("failed to restore FORGEJO_TOKEN: %v", err)
			}
		}
	}()

	for _, base := range []string{"", "   ", "\t"} {
		_, err := forgejo.NewClient(base)
		if err == nil {
			t.Fatalf("expected error for baseURL=%q, got nil", base)
		}
		if !errors.Is(err, forgejo.ErrTokenRequired) {
			t.Errorf("baseURL=%q: expected ErrTokenRequired, got: %v", base, err)
		}
	}
}

// TestErrorSentinelsAreDistinct verifies that none of the exported error sentinels
// are accidentally aliased to the same underlying value.
func TestErrorSentinelsAreDistinct(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrTokenRequired", forgejo.ErrTokenRequired},
		{"ErrInvalidURLFormat", forgejo.ErrInvalidURLFormat},
		{"ErrWorkflowTimeout", forgejo.ErrWorkflowTimeout},
		{"ErrPRNotFound", forgejo.ErrPRNotFound},
		{"ErrPRAlreadyExists", forgejo.ErrPRAlreadyExists},
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			a, b := sentinels[i], sentinels[j]
			if errors.Is(a.err, b.err) || errors.Is(b.err, a.err) {
				t.Errorf("%s and %s are identical or wrapped versions of each other", a.name, b.name)
			}
		}
	}
}

// TestLabelNameRoundtrip verifies that a Label created with a known name preserves
// that name exactly — no trimming, casing change, or transformation.
func TestLabelNameRoundtrip(t *testing.T) {
	cases := []string{"bug", "enhancement", "good first issue", "ci:skip", "v2.0"}
	for _, name := range cases {
		label := forgejo.Label{Name: name}
		if label.Name != name {
			t.Errorf("Label.Name round-trip failed: got %q, want %q", label.Name, name)
		}
	}
}
