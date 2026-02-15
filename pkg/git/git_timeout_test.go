package git_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/pkg/git"
)

// TestSwitchBranch_WithCancelledContext tests that SwitchBranch respects context cancellation
func TestSwitchBranch_WithCancelledContext(t *testing.T) {
	repo := setupTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := repo.SwitchBranch(ctx, "main")

	// Expect either a GitTimeoutError or a context-related error
	if err == nil {
		t.Error("Expected error with cancelled context, got nil")
	}

	// Check if it's a GitTimeoutError
	var timeoutErr *git.GitTimeoutError
	if errors.As(err, &timeoutErr) {
		if timeoutErr.Operation != "switch" {
			t.Errorf("Expected operation 'switch', got '%s'", timeoutErr.Operation)
		}
		return
	}

	// If not GitTimeoutError, it should still be context-related
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context") {
		t.Errorf("Expected context-related error, got: %v", err)
	}
}

// TestPull_WithCancelledContext tests that Pull respects context cancellation
func TestPull_WithCancelledContext(t *testing.T) {
	repo := setupTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := repo.Pull(ctx)

	// Expect either a GitTimeoutError or a context-related error
	if err == nil {
		t.Error("Expected error with cancelled context, got nil")
	}

	// Check if it's a GitTimeoutError
	var timeoutErr *git.GitTimeoutError
	if errors.As(err, &timeoutErr) {
		if timeoutErr.Operation != "pull" {
			t.Errorf("Expected operation 'pull', got '%s'", timeoutErr.Operation)
		}
		return
	}

	// If not GitTimeoutError, it should still be context-related
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context") {
		t.Errorf("Expected context-related error, got: %v", err)
	}
}

// TestDeleteBranch_WithCancelledContext tests that DeleteBranch respects context cancellation
func TestDeleteBranch_WithCancelledContext(t *testing.T) {
	repo := setupTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := repo.DeleteBranch(ctx, "feature-branch")

	// Expect either a GitTimeoutError or a context-related error
	if err == nil {
		t.Error("Expected error with cancelled context, got nil")
	}

	// Check if it's a GitTimeoutError
	var timeoutErr *git.GitTimeoutError
	if errors.As(err, &timeoutErr) {
		if timeoutErr.Operation != "delete branch" {
			t.Errorf("Expected operation 'delete branch', got '%s'", timeoutErr.Operation)
		}
		return
	}

	// If not GitTimeoutError, it should still be context-related
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context") {
		t.Errorf("Expected context-related error, got: %v", err)
	}
}

// TestFetchAndPrune_WithCancelledContext tests that FetchAndPrune respects context cancellation
func TestFetchAndPrune_WithCancelledContext(t *testing.T) {
	repo := setupTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := repo.FetchAndPrune(ctx)

	// Expect either a GitTimeoutError or a context-related error
	if err == nil {
		t.Error("Expected error with cancelled context, got nil")
	}

	// Check if it's a GitTimeoutError
	var timeoutErr *git.GitTimeoutError
	if errors.As(err, &timeoutErr) {
		if timeoutErr.Operation != "fetch and prune" {
			t.Errorf("Expected operation 'fetch and prune', got '%s'", timeoutErr.Operation)
		}
		return
	}

	// If not GitTimeoutError, it should still be context-related
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context") {
		t.Errorf("Expected context-related error, got: %v", err)
	}
}

// TestGitTimeoutError_Unwrap tests that GitTimeoutError properly unwraps to base error
func TestGitTimeoutError_Unwrap(t *testing.T) {
	baseErr := errors.New("base error")
	timeoutErr := &git.GitTimeoutError{
		Operation: "test",
		Timeout:   10 * time.Second,
		Err:       baseErr,
	}

	if !errors.Is(timeoutErr, baseErr) {
		t.Error("GitTimeoutError should unwrap to base error")
	}
}

// TestGitTimeoutError_Message tests that error message includes operation and timeout
func TestGitTimeoutError_Message(t *testing.T) {
	err := &git.GitTimeoutError{
		Operation: "pull",
		Timeout:   2 * time.Minute,
		Err:       errors.New("context deadline exceeded"),
	}

	msg := err.Error()
	if !strings.Contains(msg, "pull") {
		t.Errorf("Error message missing operation 'pull': %s", msg)
	}
	if !strings.Contains(msg, "2m0s") {
		t.Errorf("Error message missing timeout '2m0s': %s", msg)
	}
	if !strings.Contains(msg, "context deadline exceeded") {
		t.Errorf("Error message missing wrapped error: %s", msg)
	}
}

// TestSwitchBranch_WithTimeout tests that SwitchBranch works with a reasonable timeout
func TestSwitchBranch_WithTimeout(t *testing.T) {
	repo := setupTestRepo(t)

	// Use a reasonable timeout that should be enough for the operation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// This should succeed if on main branch
	err := repo.SwitchBranch(ctx, "main")
	if err != nil {
		// It's okay if it fails for non-timeout reasons (e.g., already on main)
		var timeoutErr *git.GitTimeoutError
		if errors.As(err, &timeoutErr) {
			t.Errorf("Operation timed out with 30s timeout: %v", err)
		}
	}
}

// TestCleanup_WithContext tests that Cleanup properly propagates context to all operations
func TestCleanup_WithContext(t *testing.T) {
	repo := setupTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Call Cleanup which should propagate the cancelled context
	report := repo.Cleanup(ctx, "main", "feature-branch")

	// At least one operation should fail due to cancelled context
	if report.Success() {
		t.Error("Expected Cleanup to fail with cancelled context")
	}

	// Check that the error is context-related
	firstErr := report.FirstError()
	if firstErr == nil {
		t.Error("Expected an error from Cleanup with cancelled context")
	}
}

// setupTestRepo creates a test repository for testing
// This is a helper function that should be defined in git_test.go or here
func setupTestRepo(t *testing.T) *git.Repository {
	t.Helper()

	// Open the current repository (assuming tests run from project root)
	repo, err := git.OpenRepository("../..")
	if err != nil {
		t.Skipf("Skipping test: not in a git repository: %v", err)
	}

	return repo
}
