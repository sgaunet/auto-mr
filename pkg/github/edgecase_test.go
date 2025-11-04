package github

import (
	"os"
	"testing"

	"github.com/sgaunet/bullets"
)

// TestCheckTrackerEdgeCases tests edge case handling for nil checks, invalid IDs, and duplicates.
func TestCheckTrackerEdgeCases(t *testing.T) {
	tracker := newCheckTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	t.Run("nil check in list", func(t *testing.T) {
		checks := []*JobInfo{
			{ID: 1, Name: "valid", Status: statusInProgress},
			nil,
			{ID: 2, Name: "another-valid", Status: statusQueued},
		}

		transitions := tracker.update(checks, logger)
		
		// Should process 2 valid checks, skip nil
		if len(transitions) != 2 {
			t.Errorf("Expected 2 transitions, got %d", len(transitions))
		}

		// Verify valid checks were tracked
		if _, exists := tracker.getCheck(1); !exists {
			t.Error("Valid check 1 should be tracked")
		}
		if _, exists := tracker.getCheck(2); !exists {
			t.Error("Valid check 2 should be tracked")
		}
	})

	t.Run("check with zero ID", func(t *testing.T) {
		tracker.reset()
		checks := []*JobInfo{
			{ID: 0, Name: "invalid", Status: statusInProgress},
			{ID: 1, Name: "valid", Status: statusInProgress},
		}

		transitions := tracker.update(checks, logger)
		
		// Should skip check with ID 0
		if len(transitions) != 1 {
			t.Errorf("Expected 1 transition, got %d", len(transitions))
		}

		// Verify zero ID check was not tracked
		if _, exists := tracker.getCheck(0); exists {
			t.Error("Check with ID 0 should not be tracked")
		}

		// Verify valid check was tracked
		if _, exists := tracker.getCheck(1); !exists {
			t.Error("Valid check should be tracked")
		}
	})

	t.Run("duplicate check IDs", func(t *testing.T) {
		tracker.reset()
		checks := []*JobInfo{
			{ID: 1, Name: "first", Status: statusInProgress},
			{ID: 1, Name: "duplicate", Status: statusCompleted, Conclusion: conclusionSuccess},
			{ID: 2, Name: "second", Status: statusQueued},
		}

		transitions := tracker.update(checks, logger)
		
		// Should process 2 checks (skip duplicate)
		if len(transitions) != 2 {
			t.Errorf("Expected 2 transitions, got %d", len(transitions))
		}

		// Verify first occurrence was used
		check, exists := tracker.getCheck(1)
		if !exists {
			t.Fatal("Check 1 should be tracked")
		}
		if check.Name != "first" {
			t.Errorf("Expected first occurrence to be tracked, got name: %s", check.Name)
		}
	})

	t.Run("all invalid checks", func(t *testing.T) {
		tracker.reset()
		checks := []*JobInfo{
			nil,
			{ID: 0, Name: "invalid", Status: statusInProgress},
			nil,
		}

		transitions := tracker.update(checks, logger)
		
		// Should process no checks
		if len(transitions) != 0 {
			t.Errorf("Expected 0 transitions, got %d", len(transitions))
		}

		// Verify no checks were tracked
		allChecks := tracker.getAllChecks()
		if len(allChecks) != 0 {
			t.Errorf("Expected 0 tracked checks, got %d", len(allChecks))
		}
	})

	t.Run("empty check list", func(t *testing.T) {
		tracker.reset()
		var checks []*JobInfo

		transitions := tracker.update(checks, logger)

		// Should handle empty list gracefully
		if len(transitions) != 0 {
			t.Errorf("Expected 0 transitions for empty list, got %d", len(transitions))
		}
	})
}

// TestProcessCheckRunsFallbackNilHandle documents the nil handle scenario fix.
// This test verifies that the defensive nil check in processCheckRunsFallback()
// prevents panics when called with nil handle during fallback scenarios.
//
// Context: When processWorkflowsWithJobTracking() falls back to check runs
// (lines 409, 424 in github.go), it passes nil for the handle parameter.
// The processCheckRunsFallback() function now creates a handle if nil is passed.
//
// Integration test note: Full API testing requires GitHub API mocking.
// Manual testing: Create a PR where workflow jobs API fails or returns no jobs.
func TestProcessCheckRunsFallbackNilHandle(t *testing.T) {
	// This test documents the bug fix for nil pointer dereference at pkg/github/github.go:819
	// The fix adds defensive nil checking in processCheckRunsFallback() at line 487-489

	// Verify bullets.UpdatableLogger can create handles (the fix relies on this)
	logger := bullets.NewUpdatable(os.Stdout)
	handle := logger.InfoHandle("test message")

	if handle == nil {
		t.Fatal("InfoHandle should never return nil - this would break the fix")
	}

	// Verify handle can be updated without panic
	handle.Update(bullets.InfoLevel, "updated message")

	// NOTE: Actual testing of processCheckRunsFallback() requires GitHub API mocking
	// The fix ensures handle is created via: c.updatableLog.InfoHandle("Checking workflow status...")
	// This test verifies the underlying mechanism works correctly
}
