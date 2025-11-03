package gitlab

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sgaunet/bullets"
)

// TestJobTrackerConcurrentAccess tests concurrent read and write operations on jobTracker.
func TestJobTrackerConcurrentAccess(t *testing.T) {
	tracker := newJobTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Create test jobs
	jobs := make([]*Job, 10)
	for i := 0; i < 10; i++ {
		jobs[i] = &Job{
			ID:     i,
			Name:   "test-job",
			Status: statusPending,
			Stage:  "test",
		}
	}

	// Run concurrent operations
	var wg sync.WaitGroup
	wg.Add(3)

	// Goroutine 1: Write jobs
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			tracker.setJob(i, jobs[i])
		}
	}()

	// Goroutine 2: Read jobs
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_, _ = tracker.getJob(i)
		}
	}()

	// Goroutine 3: Set handles
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			handle := logger.InfoHandle("test")
			tracker.setHandle(i, handle)
		}
	}()

	wg.Wait()

	// Verify all jobs were set
	for i := 0; i < 10; i++ {
		job, exists := tracker.getJob(i)
		if !exists {
			t.Errorf("Job %d should exist", i)
		}
		if job.ID != i {
			t.Errorf("Job ID mismatch: got %d, want %d", job.ID, i)
		}
	}
}

// TestJobTrackerStateTransitions tests the update method for detecting state transitions.
func TestJobTrackerStateTransitions(t *testing.T) {
	tracker := newJobTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Initial jobs
	initialJobs := []*Job{
		{ID: 1, Name: "job1", Status: statusPending, Stage: "build"},
		{ID: 2, Name: "job2", Status: statusPending, Stage: "test"},
	}

	// First update - all new jobs
	transitions := tracker.update(initialJobs, logger)
	if len(transitions) != 2 {
		t.Errorf("Expected 2 transitions (new jobs), got %d", len(transitions))
	}

	// Second update - one status change
	updatedJobs := []*Job{
		{ID: 1, Name: "job1", Status: statusRunning, Stage: "build"},
		{ID: 2, Name: "job2", Status: statusPending, Stage: "test"},
	}

	transitions = tracker.update(updatedJobs, logger)
	if len(transitions) != 1 {
		t.Errorf("Expected 1 transition (status change), got %d", len(transitions))
	}

	// Third update - completion and new job
	finalJobs := []*Job{
		{ID: 1, Name: "job1", Status: statusSuccess, Stage: "build"},
		{ID: 2, Name: "job2", Status: statusSuccess, Stage: "test"},
		{ID: 3, Name: "job3", Status: statusPending, Stage: "deploy"},
	}

	transitions = tracker.update(finalJobs, logger)
	if len(transitions) != 3 { // 2 status changes + 1 new job
		t.Errorf("Expected 3 transitions, got %d", len(transitions))
	}
}

// TestJobTrackerCleanup tests the cleanup method for removing old jobs.
func TestJobTrackerCleanup(t *testing.T) {
	tracker := newJobTracker()

	// Create jobs with different statuses
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)

	tracker.setJob(1, &Job{
		ID:         1,
		Status:     statusSuccess,
		FinishedAt: &oldTime,
	})

	tracker.setJob(2, &Job{
		ID:     2,
		Status: statusRunning,
	})

	tracker.setJob(3, &Job{
		ID:         3,
		Status:     statusFailed,
		FinishedAt: &oldTime,
	})

	// Cleanup with 1 hour retention
	tracker.cleanup(1 * time.Hour)

	// Job 1 and 3 should be cleaned up (completed and old)
	_, exists1 := tracker.getJob(1)
	if exists1 {
		t.Error("Job 1 should have been cleaned up")
	}

	// Job 2 should still exist (still running)
	_, exists2 := tracker.getJob(2)
	if !exists2 {
		t.Error("Job 2 should still exist")
	}

	_, exists3 := tracker.getJob(3)
	if exists3 {
		t.Error("Job 3 should have been cleaned up")
	}
}

// TestJobTrackerGetActiveJobs tests retrieving active jobs.
func TestJobTrackerGetActiveJobs(t *testing.T) {
	tracker := newJobTracker()

	tracker.setJob(1, &Job{ID: 1, Status: statusRunning})
	tracker.setJob(2, &Job{ID: 2, Status: statusSuccess})
	tracker.setJob(3, &Job{ID: 3, Status: statusPending})
	tracker.setJob(4, &Job{ID: 4, Status: statusFailed})

	active := tracker.getActiveJobs()
	if len(active) != 2 { // Should have running and pending
		t.Errorf("Expected 2 active jobs, got %d", len(active))
	}
}

// TestJobTrackerGetFailedJobs tests retrieving failed jobs.
func TestJobTrackerGetFailedJobs(t *testing.T) {
	tracker := newJobTracker()

	tracker.setJob(1, &Job{ID: 1, Status: statusRunning})
	tracker.setJob(2, &Job{ID: 2, Status: statusFailed})
	tracker.setJob(3, &Job{ID: 3, Status: statusFailed})
	tracker.setJob(4, &Job{ID: 4, Status: statusSuccess})

	failed := tracker.getFailedJobs()
	if len(failed) != 2 {
		t.Errorf("Expected 2 failed jobs, got %d", len(failed))
	}
}

// TestJobTrackerReset tests the reset method.
func TestJobTrackerReset(t *testing.T) {
	tracker := newJobTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Add some jobs and handles
	for i := 0; i < 5; i++ {
		tracker.setJob(i, &Job{ID: i})
		handle := logger.InfoHandle("test")
		tracker.setHandle(i, handle)
	}

	// Reset
	tracker.reset()

	// Verify all jobs and handles are gone
	allJobs := tracker.getAllJobs()
	if len(allJobs) != 0 {
		t.Errorf("Expected 0 jobs after reset, got %d", len(allJobs))
	}
}

// TestJobTrackerConcurrentRaceConditions runs with race detector to find race conditions.
func TestJobTrackerConcurrentRaceConditions(t *testing.T) {
	// Run with: go test -race
	tracker := newJobTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent updates
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				jobs := []*Job{
					{ID: id, Name: "job", Status: statusRunning, Stage: "test"},
				}
				tracker.update(jobs, logger)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = tracker.getActiveJobs()
				_ = tracker.getFailedJobs()
				_ = tracker.getAllJobs()
			}
		}()
	}

	wg.Wait()
}

// TestJobTrackerJobDisappearance tests handling of jobs that disappear mid-pipeline.
func TestJobTrackerJobDisappearance(t *testing.T) {
	tracker := newJobTracker()
	logger := bullets.NewUpdatable(os.Stdout)

	// Initial jobs
	initialJobs := []*Job{
		{ID: 1, Name: "job1", Status: statusRunning, Stage: "build"},
		{ID: 2, Name: "job2", Status: statusRunning, Stage: "test"},
		{ID: 3, Name: "job3", Status: statusPending, Stage: "deploy"},
	}
	tracker.update(initialJobs, logger)

	// Job 2 disappears in next update
	updatedJobs := []*Job{
		{ID: 1, Name: "job1", Status: statusSuccess, Stage: "build"},
		{ID: 3, Name: "job3", Status: statusRunning, Stage: "deploy"},
	}

	transitions := tracker.update(updatedJobs, logger)

	// Should detect: job1 status change, job3 status change, job2 removed
	if len(transitions) < 1 {
		t.Error("Should detect at least one transition")
	}

	// Verify job 2 still tracked (we don't automatically remove on disappearance)
	_, exists := tracker.getJob(2)
	if !exists {
		t.Error("Job 2 should still be tracked even if not in latest update")
	}
}
