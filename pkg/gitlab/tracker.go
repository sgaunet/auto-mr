package gitlab

import (
	"fmt"

	"github.com/sgaunet/bullets"
)

// newJobTracker creates a new job tracker with initialized maps.
func newJobTracker() *jobTracker {
	return &jobTracker{
		jobs:     make(map[int]*Job),
		handles:  make(map[int]*bullets.BulletHandle),
		spinners: make(map[int]*bullets.Spinner),
	}
}

// getJob retrieves a job by ID with read lock.
func (jt *jobTracker) getJob(id int) (*Job, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	job, exists := jt.jobs[id]
	return job, exists
}

// setJob stores a job by ID with write lock.
func (jt *jobTracker) setJob(id int, job *Job) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.jobs[id] = job
}

// getHandle retrieves a bullet handle by job ID with read lock.
func (jt *jobTracker) getHandle(id int) (*bullets.BulletHandle, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	handle, exists := jt.handles[id]
	return handle, exists
}

// setHandle stores a bullet handle for a job ID with write lock.
func (jt *jobTracker) setHandle(id int, handle *bullets.BulletHandle) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.handles[id] = handle
}

// getSpinner retrieves a spinner by job ID with read lock.
func (jt *jobTracker) getSpinner(id int) (*bullets.Spinner, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	spinner, exists := jt.spinners[id]
	return spinner, exists
}

// setSpinner stores a spinner for a job ID with write lock.
func (jt *jobTracker) setSpinner(id int, spinner *bullets.Spinner) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.spinners[id] = spinner
}

// deleteSpinner stops and removes a spinner with write lock.
func (jt *jobTracker) deleteSpinner(id int) {
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
	newJobIDs := make(map[int]bool)

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

	if newJob.Status == statusRunning {
		spinner := logger.SpinnerCircle(statusText)
		jt.setSpinner(newJob.ID, spinner)
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
func (jt *jobTracker) detectRemovedJobs(newJobIDs map[int]bool) []string {
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

// transitionJobToRunning creates a spinner when a job transitions to running state.
func (jt *jobTracker) transitionJobToRunning(logger *bullets.UpdatableLogger, jobID int, statusText string) {
	// Stop any existing handle
	if handle, exists := jt.getHandle(jobID); exists {
		handle.Update(bullets.InfoLevel, "") // Clear the line
		jt.mu.Lock()
		delete(jt.handles, jobID)
		jt.mu.Unlock()
	}
	// Create animated spinner
	spinner := logger.SpinnerCircle(statusText)
	jt.setSpinner(jobID, spinner)
}

// transitionJobToNonRunning creates a handle when a job transitions from running state.
func (jt *jobTracker) transitionJobToNonRunning(logger *bullets.UpdatableLogger, jobID int, statusText string) {
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
func (jt *jobTracker) updateExistingJobDisplay(jobID int, statusText string) {
	if _, exists := jt.getSpinner(jobID); exists {
		// Spinner is running, no update needed (animation continues)
		return
	}
	if handle, exists := jt.getHandle(jobID); exists {
		// Static handle, update text
		handle.Update(bullets.InfoLevel, statusText)
	}
}
