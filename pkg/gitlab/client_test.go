package gitlab_test

import (
	"errors"
	"os"
	"testing"

	"github.com/sgaunet/auto-mr/pkg/gitlab"
)

// TestClientConstructor tests the NewClient function.
func TestClientConstructor(t *testing.T) {
	t.Run("NewClient requires GITLAB_TOKEN", func(t *testing.T) {
		t.Skip("Requires environment manipulation")
	})
}

// TestNewClientWhitespaceTokenTrimmed verifies that a whitespace-only GITLAB_TOKEN
// is trimmed to empty and reported as missing, rather than producing an invalid
// Authorization header.
func TestNewClientWhitespaceTokenTrimmed(t *testing.T) {
	original := os.Getenv("GITLAB_TOKEN")
	t.Setenv("GITLAB_TOKEN", "   \n\t ")

	defer func() {
		if original == "" {
			if err := os.Unsetenv("GITLAB_TOKEN"); err != nil {
				t.Errorf("failed to unset GITLAB_TOKEN: %v", err)
			}
			return
		}
	}()

	_, err := gitlab.NewClient()
	if !errors.Is(err, gitlab.ErrTokenRequired) {
		t.Errorf("expected ErrTokenRequired for whitespace-only token, got: %v", err)
	}
}
