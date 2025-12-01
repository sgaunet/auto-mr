package gitlab

import (
	"fmt"
	"time"

	"github.com/sgaunet/bullets"
)

// newJobTracker creates a new job tracker with initialized maps.
func newJobTracker() *jobTracker {
	return &jobTracker{
		jobs:     make(map[int64]*Job),
		handles:  make(map[int64]*bullets.BulletHandle),
		spinners: make(map[int64]*bullets.Spinner),
	}
}

// getJob retrieves a job by ID with read lock.
func (jt *jobTracker) getJob(id int64) (*Job, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	job, exists := jt.jobs[id]
	return job, exists
}

// setJob stores a job by ID with write lock.
func (jt *jobTracker) setJob(id int64, job *Job) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.jobs[id] = job
}

// getHandle retrieves a bullet handle by job ID with read lock.
func (jt *jobTracker) getHandle(id int64) (*bullets.BulletHandle, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	handle, exists := jt.handles[id]
	return handle, exists
}

// setHandle stores a bullet handle for a job ID with write lock.
func (jt *jobTracker) setHandle(id int64, handle *bullets.BulletHandle) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.handles[id] = handle
}

// getSpinner retrieves a spinner by job ID with read lock.
func (jt *jobTracker) getSpinner(id int64) (*bullets.Spinner, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	spinner, exists := jt.spinners[id]
	return spinner, exists
}

// setSpinner stores a spinner for a job ID with write lock.
func (jt *jobTracker) setSpinner(id int64, spinner *bullets.Spinner) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.spinners[id] = spinner
}

// deleteSpinner stops and removes a spinner with write lock.
func (jt *jobTracker) deleteSpinner(id int64) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	if spinner, exists := jt.spinners[id]; exists {
		spinner.Stop()
		delete(jt.spinners, id)
	}
}

// update processes new jobs, detects state transitions, and updates handles.
// Returns a list of state transition descriptions.
func (jt *jobTracker) update(newJobs []*Job, logger *bullets.UpdatableLogger) []string {
	var transitions []string
	newJobIDs := make(map[int64]bool)

	for _, newJob := range newJobs {
		if newJob == nil || newJob.ID == 0 || newJobIDs[newJob.ID] {
			continue
		}

		newJobIDs[newJob.ID] = true
		transition := jt.processJobUpdate(newJob, logger)
		if transition != "" {
			transitions = append(transitions, transition)
		}
	}

	// Detect removed jobs
	transitions = append(transitions, jt.detectRemovedJobs(newJobIDs)...)

	return transitions
}

// processJobUpdate handles the update logic for a single job.
func (jt *jobTracker) processJobUpdate(newJob *Job, logger *bullets.UpdatableLogger) string {
	oldJob, exists := jt.getJob(newJob.ID)

	switch {
	case !exists:
		return jt.handleNewJob(newJob, logger)
	case oldJob.Status != newJob.Status:
		return jt.handleJobStatusChange(oldJob, newJob, logger)
	default:
		return jt.handleJobDataUpdate(newJob)
	}
}

// handleNewJob processes a newly detected job.
func (jt *jobTracker) handleNewJob(newJob *Job, logger *bullets.UpdatableLogger) string {
	jt.setJob(newJob.ID, newJob)
	statusText := formatJobStatus(newJob)

	if newJob.Status == statusRunning || newJob.Status == statusPending {
		spinner := logger.SpinnerCircle(statusText)
		jt.setSpinner(newJob.ID, spinner)
		// Start time update loop for any job with spinner that has started timing
		if newJob.StartedAt != nil {
			go jt.updateSpinnerLoop(newJob.ID, spinner)
		}
	} else {
		handle := logger.InfoHandle(statusText)
		jt.setHandle(newJob.ID, handle)
	}

	return fmt.Sprintf("Job %d started: %s/%s", newJob.ID, newJob.Stage, newJob.Name)
}

// handleJobStatusChange processes a job with changed status.
func (jt *jobTracker) handleJobStatusChange(oldJob, newJob *Job, logger *bullets.UpdatableLogger) string {
	wasPulsing := oldJob.Status == statusRunning
	isPulsing := newJob.Status == statusRunning

	jt.updateHandleForJob(logger, newJob, wasPulsing, isPulsing)
	jt.setJob(newJob.ID, newJob)
	return fmt.Sprintf("Job %d: %s -> %s", newJob.ID, oldJob.Status, newJob.Status)
}

// handleJobDataUpdate updates job data without status change.
func (jt *jobTracker) handleJobDataUpdate(newJob *Job) string {
	jt.setJob(newJob.ID, newJob)
	// Update text only for non-running jobs (spinners display automatically)
	if newJob.Status != statusRunning {
		if handle, exists := jt.getHandle(newJob.ID); exists {
			statusText := formatJobStatus(newJob)
			handle.Update(bullets.InfoLevel, statusText)
		}
	}
	return ""
}

// detectRemovedJobs detects jobs that have been removed.
func (jt *jobTracker) detectRemovedJobs(newJobIDs map[int64]bool) []string {
	var transitions []string
	jt.mu.RLock()
	defer jt.mu.RUnlock()

	for id := range jt.jobs {
		if !newJobIDs[id] {
			transitions = append(transitions, fmt.Sprintf("Job %d removed", id))
		}
	}
	return transitions
}

// updateHandleForJob updates the display for a job when status changes.
// wasPulsing and isPulsing control whether to start or stop the spinner animation.
func (jt *jobTracker) updateHandleForJob(logger *bullets.UpdatableLogger, job *Job, wasPulsing, isPulsing bool) {
	statusText := formatJobStatus(job)

	if job.Status == statusSuccess || job.Status == statusFailed || job.Status == statusCanceled {
		jt.finalizeCompletedJob(job, statusText)
		return
	}

	if isPulsing && !wasPulsing {
		jt.transitionJobToRunning(logger, job.ID, statusText)
		return
	}

	if !isPulsing && wasPulsing {
		jt.transitionJobToNonRunning(logger, job.ID, statusText)
		return
	}

	jt.updateExistingJobDisplay(job.ID, statusText)
}

// finalizeCompletedJob handles completed jobs - finalize spinner or handle.
func (jt *jobTracker) finalizeCompletedJob(job *Job, statusText string) {
	// If was running, stop spinner with final message
	if spinner, exists := jt.getSpinner(job.ID); exists {
		jt.finalizeJobSpinner(spinner, job.Status, statusText)
		jt.deleteSpinner(job.ID)
		return
	}

	// Was not running, update handle
	if handle, exists := jt.getHandle(job.ID); exists {
		jt.finalizeJobHandle(handle, job.Status, statusText)
	}
}

// finalizeJobSpinner stops a spinner with the appropriate final message.
func (jt *jobTracker) finalizeJobSpinner(spinner *bullets.Spinner, status, statusText string) {
	switch status {
	case statusSuccess:
		spinner.Success(statusText)
	case statusCanceled:
		spinner.Replace(statusText) // Use Replace for canceled (neutral outcome)
	default:
		spinner.Error(statusText)
	}
}

// finalizeJobHandle updates a handle with the appropriate final status.
func (jt *jobTracker) finalizeJobHandle(handle *bullets.BulletHandle, status, statusText string) {
	switch status {
	case statusSuccess:
		handle.Success(statusText)
	case statusCanceled:
		handle.Warning(statusText)
	default:
		handle.Error(statusText)
	}
}

// transitionJobToRunning updates or creates a spinner when a job transitions to running state.
func (jt *jobTracker) transitionJobToRunning(logger *bullets.UpdatableLogger, jobID int64, statusText string) {
	// Check if spinner already exists
	if spinner, exists := jt.getSpinner(jobID); exists {
		// Spinner exists, just update its text (don't recreate!)
		spinner.UpdateText(statusText)
		return
	}

	// Stop any existing handle if present
	if handle, exists := jt.getHandle(jobID); exists {
		handle.Update(bullets.InfoLevel, "") // Clear the line
		jt.mu.Lock()
		delete(jt.handles, jobID)
		jt.mu.Unlock()
	}

	// Create new animated spinner (only if doesn't exist)
	spinner := logger.SpinnerCircle(statusText)
	jt.setSpinner(jobID, spinner)

	// Start time update loop for this spinner
	go jt.updateSpinnerLoop(jobID, spinner)
}

// transitionJobToNonRunning creates a handle when a job transitions from running state.
func (jt *jobTracker) transitionJobToNonRunning(logger *bullets.UpdatableLogger, jobID int64, statusText string) {
	// Stop spinner
	if spinner, exists := jt.getSpinner(jobID); exists {
		spinner.Replace(statusText)
		jt.deleteSpinner(jobID)
	}
	// Create static handle
	handle := logger.InfoHandle(statusText)
	jt.setHandle(jobID, handle)
}

// updateExistingJobDisplay updates existing display without animation state change.
func (jt *jobTracker) updateExistingJobDisplay(jobID int64, statusText string) {
	// Check for spinner first
	if spinner, exists := jt.getSpinner(jobID); exists {
		// Spinner exists, update its text (CHANGED: was early return)
		spinner.UpdateText(statusText)
		return
	}

	// Static handle, update text
	if handle, exists := jt.getHandle(jobID); exists {
		handle.Update(bullets.InfoLevel, statusText)
	}
}

// updateSpinnerLoop continuously updates spinner text with current elapsed time.
// Runs in a background goroutine for jobs with StartedAt timestamps.
// Terminates when job completes or spinner is removed.
func (jt *jobTracker) updateSpinnerLoop(jobID int64, spinner *bullets.Spinner) {
	ticker := time.NewTicker(spinnerUpdateInterval)
	defer ticker.Stop()

	for range ticker.C {
		job, exists := jt.getJob(jobID)

		// Stop if job no longer exists
		if !exists {
			break
		}

		// Stop if job completed (will be finalized by tracker)
		if job.Status == statusSuccess || job.Status == statusFailed ||
			job.Status == statusCanceled {
			break
		}

		// Stop if spinner was removed (shouldn't happen, but defensive)
		if _, spinnerExists := jt.getSpinner(jobID); !spinnerExists {
			break
		}

		// Update spinner text with fresh duration calculation
		statusText := formatJobStatus(job)
		spinner.UpdateText(statusText)
	}
}
