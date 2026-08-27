package github_test

import (
	"errors"
	"os"
	"testing"

	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
)

// TestClientConstructor tests the client construction and configuration.
func TestClientConstructor(t *testing.T) {
	t.Run("NewClient requires GITHUB_TOKEN", func(t *testing.T) {
		// This would require unsetting env var, which is tricky in tests
		// Skip for now as it would affect other tests
		t.Skip("Requires environment manipulation")
	})
}

// TestNewClientWhitespaceTokenTrimmed verifies that a whitespace-only GITHUB_TOKEN
// is trimmed to empty and reported as missing, rather than producing an invalid
// Authorization header.
func TestNewClientWhitespaceTokenTrimmed(t *testing.T) {
	original := os.Getenv("GITHUB_TOKEN")
	if err := os.Setenv("GITHUB_TOKEN", "   \n\t "); err != nil {
		t.Fatalf("failed to set GITHUB_TOKEN: %v", err)
	}

	defer func() {
		if original == "" {
			if err := os.Unsetenv("GITHUB_TOKEN"); err != nil {
				t.Errorf("failed to unset GITHUB_TOKEN: %v", err)
			}
			return
		}
		if err := os.Setenv("GITHUB_TOKEN", original); err != nil {
			t.Errorf("failed to restore GITHUB_TOKEN: %v", err)
		}
	}()

	_, err := ghpkg.NewClient(t.Context())
	if !errors.Is(err, ghpkg.ErrTokenRequired) {
		t.Errorf("expected ErrTokenRequired for whitespace-only token, got: %v", err)
	}
}

// TestGetMergeMethod tests the merge method utility function.
func TestGetMergeMethod(t *testing.T) {
	tests := []struct {
		name   string
		squash bool
		want   string
	}{
		{
			name:   "squash merge",
			squash: true,
			want:   "squash",
		},
		{
			name:   "regular merge",
			squash: false,
			want:   "merge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ghpkg.GetMergeMethod(tt.squash)
			if got != tt.want {
				t.Errorf("GetMergeMethod(%v) = %v, want %v", tt.squash, got, tt.want)
			}
		})
	}
}
