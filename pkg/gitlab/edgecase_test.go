package gitlab

import (
	"os"
	"testing"

	"github.com/sgaunet/bullets"
)

// TestJobTrackerEdgeCases tests edge case handling for nil jobs, invalid IDs, and duplicates.
func TestJobTrackerEdgeCases(t *testing.T) {
	tracker := newJobTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	t.Run("nil job in list", func(t *testing.T) {
		jobs := []*Job{
			{ID: 1, Name: "valid", Status: statusRunning},
			nil,
			{ID: 2, Name: "another-valid", Status: statusPending},
		}

		transitions := tracker.update(jobs, logger)
		
		// Should process 2 valid jobs, skip nil
		if len(transitions) != 2 {
			t.Errorf("Expected 2 transitions, got %d", len(transitions))
		}

		// Verify valid jobs were tracked
		if _, exists := tracker.getJob(1); !exists {
			t.Error("Valid job 1 should be tracked")
		}
		if _, exists := tracker.getJob(2); !exists {
			t.Error("Valid job 2 should be tracked")
		}
	})

	t.Run("job with zero ID", func(t *testing.T) {
		tracker.reset()
		jobs := []*Job{
			{ID: 0, Name: "invalid", Status: statusRunning},
			{ID: 1, Name: "valid", Status: statusRunning},
		}

		transitions := tracker.update(jobs, logger)
		
		// Should skip job with ID 0
		if len(transitions) != 1 {
			t.Errorf("Expected 1 transition, got %d", len(transitions))
		}

		// Verify zero ID job was not tracked
		if _, exists := tracker.getJob(0); exists {
			t.Error("Job with ID 0 should not be tracked")
		}

		// Verify valid job was tracked
		if _, exists := tracker.getJob(1); !exists {
			t.Error("Valid job should be tracked")
		}
	})

	t.Run("duplicate job IDs", func(t *testing.T) {
		tracker.reset()
		jobs := []*Job{
			{ID: 1, Name: "first", Status: statusRunning},
			{ID: 1, Name: "duplicate", Status: statusSuccess},
			{ID: 2, Name: "second", Status: statusPending},
		}

		transitions := tracker.update(jobs, logger)
		
		// Should process 2 jobs (skip duplicate)
		if len(transitions) != 2 {
			t.Errorf("Expected 2 transitions, got %d", len(transitions))
		}

		// Verify first occurrence was used
		job, exists := tracker.getJob(1)
		if !exists {
			t.Fatal("Job 1 should be tracked")
		}
		if job.Name != "first" {
			t.Errorf("Expected first occurrence to be tracked, got name: %s", job.Name)
		}
	})

	t.Run("all invalid jobs", func(t *testing.T) {
		tracker.reset()
		jobs := []*Job{
			nil,
			{ID: 0, Name: "invalid", Status: statusRunning},
			nil,
		}

		transitions := tracker.update(jobs, logger)
		
		// Should process no jobs
		if len(transitions) != 0 {
			t.Errorf("Expected 0 transitions, got %d", len(transitions))
		}

		// Verify no jobs were tracked
		allJobs := tracker.getAllJobs()
		if len(allJobs) != 0 {
			t.Errorf("Expected 0 tracked jobs, got %d", len(allJobs))
		}
	})

	t.Run("empty job list", func(t *testing.T) {
		tracker.reset()
		var jobs []*Job

		transitions := tracker.update(jobs, logger)
		
		// Should handle empty list gracefully
		if len(transitions) != 0 {
			t.Errorf("Expected 0 transitions for empty list, got %d", len(transitions))
		}
	})
}
