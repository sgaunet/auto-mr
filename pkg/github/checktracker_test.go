package github

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sgaunet/bullets"
)

// TestCheckTrackerConcurrentAccess tests concurrent read and write operations on checkTracker.
func TestCheckTrackerConcurrentAccess(t *testing.T) {
	tracker := newCheckTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Create test checks
	checks := make([]*JobInfo, 10)
	for i := 0; i < 10; i++ {
		checks[i] = &JobInfo{
			ID:     int64(i),
			Name:   "test-check",
			Status: statusQueued,
		}
	}

	// Run concurrent operations
	var wg sync.WaitGroup
	wg.Add(3)

	// Goroutine 1: Write checks
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			tracker.setCheck(int64(i), checks[i])
		}
	}()

	// Goroutine 2: Read checks
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_, _ = tracker.getCheck(int64(i))
		}
	}()

	// Goroutine 3: Set handles
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			handle := logger.InfoHandle("test")
			tracker.setHandle(int64(i), handle)
		}
	}()

	wg.Wait()

	// Verify all checks were set
	for i := 0; i < 10; i++ {
		check, exists := tracker.getCheck(int64(i))
		if !exists {
			t.Errorf("Check %d should exist", i)
		}
		if check.ID != int64(i) {
			t.Errorf("Check ID mismatch: got %d, want %d", check.ID, i)
		}
	}
}

// TestCheckTrackerStateTransitions tests the update method for detecting state transitions.
func TestCheckTrackerStateTransitions(t *testing.T) {
	tracker := newCheckTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Initial checks
	initialChecks := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusQueued},
		{ID: 2, Name: "check2", Status: statusQueued},
	}

	// First update - all new checks
	transitions := tracker.update(initialChecks, logger)
	if len(transitions) != 2 {
		t.Errorf("Expected 2 transitions (new checks), got %d", len(transitions))
	}

	// Second update - one status change
	updatedChecks := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusInProgress},
		{ID: 2, Name: "check2", Status: statusQueued},
	}

	transitions = tracker.update(updatedChecks, logger)
	if len(transitions) != 1 {
		t.Errorf("Expected 1 transition (status change), got %d", len(transitions))
	}

	// Third update - completion with conclusions
	finalChecks := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusCompleted, Conclusion: conclusionSuccess},
		{ID: 2, Name: "check2", Status: statusCompleted, Conclusion: "failure"},
		{ID: 3, Name: "check3", Status: statusQueued},
	}

	transitions = tracker.update(finalChecks, logger)
	if len(transitions) != 3 { // 2 status changes + 1 new check
		t.Errorf("Expected 3 transitions, got %d", len(transitions))
	}
}

// TestCheckTrackerConclusionChanges tests detecting conclusion changes for completed checks.
func TestCheckTrackerConclusionChanges(t *testing.T) {
	tracker := newCheckTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Initial check
	initialChecks := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusCompleted, Conclusion: conclusionSuccess},
	}
	tracker.update(initialChecks, logger)

	// Update with different conclusion (re-run scenario)
	updatedChecks := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusCompleted, Conclusion: "failure"},
	}

	transitions := tracker.update(updatedChecks, logger)
	if len(transitions) != 1 {
		t.Errorf("Expected 1 transition for conclusion change, got %d", len(transitions))
	}
}

// TestCheckTrackerCleanup tests the cleanup method for removing old checks.
func TestCheckTrackerCleanup(t *testing.T) {
	tracker := newCheckTracker()

	// Create checks with different statuses
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)

	tracker.setCheck(1, &JobInfo{
		ID:          1,
		Status:      statusCompleted,
		Conclusion:  conclusionSuccess,
		CompletedAt: &oldTime,
	})

	tracker.setCheck(2, &JobInfo{
		ID:     2,
		Status: statusInProgress,
	})

	tracker.setCheck(3, &JobInfo{
		ID:          3,
		Status:      statusCompleted,
		Conclusion:  "failure",
		CompletedAt: &oldTime,
	})

	// Cleanup with 1 hour retention
	tracker.cleanup(1 * time.Hour)

	// Check 1 and 3 should be cleaned up (completed and old)
	_, exists1 := tracker.getCheck(1)
	if exists1 {
		t.Error("Check 1 should have been cleaned up")
	}

	// Check 2 should still exist (still in progress)
	_, exists2 := tracker.getCheck(2)
	if !exists2 {
		t.Error("Check 2 should still exist")
	}

	_, exists3 := tracker.getCheck(3)
	if exists3 {
		t.Error("Check 3 should have been cleaned up")
	}
}

// TestCheckTrackerGetActiveChecks tests retrieving active checks.
func TestCheckTrackerGetActiveChecks(t *testing.T) {
	tracker := newCheckTracker()

	tracker.setCheck(1, &JobInfo{ID: 1, Status: statusInProgress})
	tracker.setCheck(2, &JobInfo{ID: 2, Status: statusCompleted, Conclusion: conclusionSuccess})
	tracker.setCheck(3, &JobInfo{ID: 3, Status: statusQueued})
	tracker.setCheck(4, &JobInfo{ID: 4, Status: statusCompleted, Conclusion: "failure"})

	active := tracker.getActiveChecks()
	if len(active) != 2 { // Should have in_progress and queued
		t.Errorf("Expected 2 active checks, got %d", len(active))
	}
}

// TestCheckTrackerGetFailedChecks tests retrieving failed checks.
func TestCheckTrackerGetFailedChecks(t *testing.T) {
	tracker := newCheckTracker()

	tracker.setCheck(1, &JobInfo{ID: 1, Status: statusInProgress})
	tracker.setCheck(2, &JobInfo{ID: 2, Status: statusCompleted, Conclusion: "failure"})
	tracker.setCheck(3, &JobInfo{ID: 3, Status: statusCompleted, Conclusion: "cancelled"})
	tracker.setCheck(4, &JobInfo{ID: 4, Status: statusCompleted, Conclusion: conclusionSuccess})
	tracker.setCheck(5, &JobInfo{ID: 5, Status: statusCompleted, Conclusion: conclusionSkipped})

	failed := tracker.getFailedChecks()
	if len(failed) != 2 { // Should have failure and cancelled
		t.Errorf("Expected 2 failed checks, got %d", len(failed))
	}
}

// TestCheckTrackerReset tests the reset method.
func TestCheckTrackerReset(t *testing.T) {
	tracker := newCheckTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Add some checks and handles
	for i := 0; i < 5; i++ {
		tracker.setCheck(int64(i), &JobInfo{ID: int64(i)})
		handle := logger.InfoHandle("test")
		tracker.setHandle(int64(i), handle)
	}

	// Reset
	tracker.reset()

	// Verify all checks and handles are gone
	allChecks := tracker.getAllChecks()
	if len(allChecks) != 0 {
		t.Errorf("Expected 0 checks after reset, got %d", len(allChecks))
	}
}

// TestCheckTrackerConcurrentRaceConditions runs with race detector to find race conditions.
func TestCheckTrackerConcurrentRaceConditions(t *testing.T) {
	// Run with: go test -race
	tracker := newCheckTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent updates
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				checks := []*JobInfo{
					{ID: id, Name: "check", Status: statusInProgress},
				}
				tracker.update(checks, logger)
			}
		}(int64(i))
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = tracker.getActiveChecks()
				_ = tracker.getFailedChecks()
				_ = tracker.getAllChecks()
			}
		}()
	}

	wg.Wait()
}

// TestCheckTrackerCheckDisappearance tests handling of checks that disappear mid-workflow.
func TestCheckTrackerCheckDisappearance(t *testing.T) {
	tracker := newCheckTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Initial checks
	initialChecks := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusInProgress},
		{ID: 2, Name: "check2", Status: statusInProgress},
		{ID: 3, Name: "check3", Status: statusQueued},
	}
	tracker.update(initialChecks, logger)

	// Check 2 disappears in next update
	updatedChecks := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusCompleted, Conclusion: conclusionSuccess},
		{ID: 3, Name: "check3", Status: statusInProgress},
	}

	transitions := tracker.update(updatedChecks, logger)

	// Should detect: check1 completion, check3 status change, check2 removed
	if len(transitions) < 1 {
		t.Error("Should detect at least one transition")
	}

	// Verify check 2 still tracked (we don't automatically remove on disappearance)
	_, exists := tracker.getCheck(2)
	if !exists {
		t.Error("Check 2 should still be tracked even if not in latest update")
	}
}

// TestCheckTrackerNeutralAndSkipped tests handling of neutral and skipped conclusions.
func TestCheckTrackerNeutralAndSkipped(t *testing.T) {
	tracker := newCheckTracker()

	tracker.setCheck(1, &JobInfo{ID: 1, Status: statusCompleted, Conclusion: conclusionNeutral})
	tracker.setCheck(2, &JobInfo{ID: 2, Status: statusCompleted, Conclusion: conclusionSkipped})
	tracker.setCheck(3, &JobInfo{ID: 3, Status: statusCompleted, Conclusion: "failure"})

	// Neutral and skipped should not be considered failed
	failed := tracker.getFailedChecks()
	if len(failed) != 1 {
		t.Errorf("Expected 1 failed check (only failure), got %d", len(failed))
	}

	// They should not be active either
	active := tracker.getActiveChecks()
	if len(active) != 0 {
		t.Errorf("Expected 0 active checks, got %d", len(active))
	}
}

// TestCheckTrackerHandleLifecycle tests handle creation and updates throughout job lifecycle.
func TestCheckTrackerHandleLifecycle(t *testing.T) {
	tracker := newCheckTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// New check - should create handle
	checks1 := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusQueued},
	}
	tracker.update(checks1, logger)

	_, hasHandle := tracker.getHandle(1)
	if !hasHandle {
		t.Error("Handle should be created for new check")
	}

	// Status change to running - should create spinner (not handle)
	checks2 := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusInProgress},
	}
	tracker.update(checks2, logger)

	_, hasSpinner := tracker.getSpinner(1)
	if !hasSpinner {
		t.Error("Spinner should exist for running check")
	}
	// Handle should be removed when spinner is created
	_, hasHandle = tracker.getHandle(1)
	if hasHandle {
		t.Error("Handle should not exist for running check (replaced by spinner)")
	}

	// Completion - spinner should be stopped and replaced with final message
	checks3 := []*JobInfo{
		{ID: 1, Name: "check1", Status: statusCompleted, Conclusion: conclusionSuccess},
	}
	tracker.update(checks3, logger)

	_, hasSpinner = tracker.getSpinner(1)
	if hasSpinner {
		t.Error("Spinner should be removed after completion")
	}
}
