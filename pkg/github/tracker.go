package github

import (
	"fmt"

	"github.com/sgaunet/bullets"
)

// newCheckTracker creates a new check tracker with initialized maps.
func newCheckTracker() *checkTracker {
	return &checkTracker{
		checks:   make(map[int64]*JobInfo),
		handles:  make(map[int64]*bullets.BulletHandle),
		spinners: make(map[int64]*bullets.Spinner),
	}
}

// getCheck retrieves a job/check by ID with read lock.
func (ct *checkTracker) getCheck(id int64) (*JobInfo, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	check, exists := ct.checks[id]
	return check, exists
}

// setCheck stores a job/check by ID with write lock.
func (ct *checkTracker) setCheck(id int64, check *JobInfo) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.checks[id] = check
}

// getHandle retrieves a bullet handle by job/check ID with read lock.
func (ct *checkTracker) getHandle(id int64) (*bullets.BulletHandle, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	handle, exists := ct.handles[id]
	return handle, exists
}

// setHandle stores a bullet handle for a job/check ID with write lock.
func (ct *checkTracker) setHandle(id int64, handle *bullets.BulletHandle) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.handles[id] = handle
}

// getSpinner retrieves a spinner by ID with read lock.
func (ct *checkTracker) getSpinner(id int64) (*bullets.Spinner, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	spinner, exists := ct.spinners[id]
	return spinner, exists
}

// setSpinner stores a spinner for a job/check ID with write lock.
func (ct *checkTracker) setSpinner(id int64, spinner *bullets.Spinner) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.spinners[id] = spinner
}

// deleteSpinner removes a spinner with write lock.
func (ct *checkTracker) deleteSpinner(id int64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if spinner, exists := ct.spinners[id]; exists {
		spinner.Stop() // Stop animation before deleting
		delete(ct.spinners, id)
	}
}

// update processes new jobs/checks, detects state transitions, and updates handles.
// Returns a list of state transition descriptions.
func (ct *checkTracker) update(newChecks []*JobInfo, logger *bullets.UpdatableLogger) []string {
	var transitions []string
	newCheckIDs := make(map[int64]bool)

	for _, newCheck := range newChecks {
		if newCheck == nil || newCheck.ID == 0 || newCheckIDs[newCheck.ID] {
			continue
		}

		newCheckIDs[newCheck.ID] = true
		transition := ct.processCheckUpdate(newCheck, logger)
		if transition != "" {
			transitions = append(transitions, transition)
		}
	}

	// Detect removed jobs
	transitions = append(transitions, ct.detectRemovedChecks(newCheckIDs)...)

	return transitions
}

// processCheckUpdate handles the update logic for a single check.
func (ct *checkTracker) processCheckUpdate(newCheck *JobInfo, logger *bullets.UpdatableLogger) string {
	oldCheck, exists := ct.getCheck(newCheck.ID)

	switch {
	case !exists:
		return ct.handleNewCheck(newCheck, logger)
	case ct.hasStatusChanged(oldCheck, newCheck):
		return ct.handleCheckStatusChange(oldCheck, newCheck, logger)
	default:
		ct.setCheck(newCheck.ID, newCheck)
		return ""
	}
}

// handleNewCheck processes a newly detected check.
func (ct *checkTracker) handleNewCheck(newCheck *JobInfo, logger *bullets.UpdatableLogger) string {
	ct.setCheck(newCheck.ID, newCheck)
	statusText := formatJobStatus(newCheck)

	if newCheck.Status == statusInProgress {
		spinner := logger.SpinnerCircle(statusText)
		ct.setSpinner(newCheck.ID, spinner)
	} else {
		handle := logger.InfoHandle(statusText)
		ct.setHandle(newCheck.ID, handle)
	}

	return fmt.Sprintf("Job %d started: %s", newCheck.ID, newCheck.Name)
}

// handleCheckStatusChange processes a check with changed status.
func (ct *checkTracker) handleCheckStatusChange(
	oldCheck, newCheck *JobInfo, logger *bullets.UpdatableLogger,
) string {
	wasPulsing := oldCheck.Status == statusInProgress
	isPulsing := newCheck.Status == statusInProgress

	ct.updateHandleForCheck(logger, newCheck, wasPulsing, isPulsing)
	ct.setCheck(newCheck.ID, newCheck)
	return ct.formatTransition(oldCheck, newCheck)
}

// detectRemovedChecks detects checks that have been removed.
func (ct *checkTracker) detectRemovedChecks(newCheckIDs map[int64]bool) []string {
	var transitions []string
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	for id := range ct.checks {
		if !newCheckIDs[id] {
			transitions = append(transitions, fmt.Sprintf("Job %d removed", id))
		}
	}
	return transitions
}

// hasStatusChanged checks if job status or conclusion changed.
func (ct *checkTracker) hasStatusChanged(oldCheck, newCheck *JobInfo) bool {
	return oldCheck.Status != newCheck.Status || oldCheck.Conclusion != newCheck.Conclusion
}

// formatTransition creates a transition message for status changes.
func (ct *checkTracker) formatTransition(oldCheck, newCheck *JobInfo) string {
	oldState := oldCheck.Status
	if oldCheck.Status == statusCompleted && oldCheck.Conclusion != "" {
		oldState = oldCheck.Conclusion
	}

	newState := newCheck.Status
	if newCheck.Status == statusCompleted && newCheck.Conclusion != "" {
		newState = newCheck.Conclusion
	}

	return fmt.Sprintf("Job %d: %s -> %s", newCheck.ID, oldState, newState)
}

// updateHandleForCheck updates display based on job status transitions.
// Manages transitions between static handles (queued) and animated spinners (running).
func (ct *checkTracker) updateHandleForCheck(
	logger *bullets.UpdatableLogger, check *JobInfo, wasPulsing, isPulsing bool,
) {
	statusText := formatJobStatus(check)

	if check.Status == statusCompleted {
		ct.finalizeCompletedCheck(check, statusText)
		return
	}

	if isPulsing && !wasPulsing {
		ct.transitionCheckToRunning(logger, check.ID, statusText)
		return
	}

	if !isPulsing && wasPulsing {
		ct.transitionCheckToNonRunning(logger, check.ID, statusText)
		return
	}

	ct.updateExistingCheckDisplay(check.ID, statusText)
}

// finalizeCompletedCheck handles completed jobs - finalize spinner or handle.
func (ct *checkTracker) finalizeCompletedCheck(check *JobInfo, statusText string) {
	// If was running, stop spinner with final message
	if spinner, exists := ct.getSpinner(check.ID); exists {
		ct.finalizeSpinner(spinner, check.Conclusion, statusText)
		ct.deleteSpinner(check.ID)
		return
	}

	// Was not running, update handle
	if handle, exists := ct.getHandle(check.ID); exists {
		ct.finalizeHandle(handle, check.Conclusion, statusText)
	}
}

// finalizeSpinner stops a spinner with the appropriate final message.
func (ct *checkTracker) finalizeSpinner(spinner *bullets.Spinner, conclusion, statusText string) {
	switch conclusion {
	case conclusionSuccess:
		spinner.Success(statusText)
	case conclusionSkipped, conclusionNeutral:
		spinner.Replace(statusText)
	default:
		spinner.Error(statusText)
	}
}

// finalizeHandle updates a handle with the appropriate final status.
func (ct *checkTracker) finalizeHandle(handle *bullets.BulletHandle, conclusion, statusText string) {
	switch conclusion {
	case conclusionSuccess:
		handle.Success(statusText)
	case conclusionSkipped, conclusionNeutral:
		handle.Update(bullets.InfoLevel, statusText)
	default:
		handle.Error(statusText)
	}
}

// transitionCheckToRunning creates a spinner when a check transitions to running state.
func (ct *checkTracker) transitionCheckToRunning(logger *bullets.UpdatableLogger, checkID int64, statusText string) {
	// Stop any existing handle
	if handle, exists := ct.getHandle(checkID); exists {
		handle.Update(bullets.InfoLevel, "") // Clear the line
		ct.mu.Lock()
		delete(ct.handles, checkID)
		ct.mu.Unlock()
	}
	// Create animated spinner
	spinner := logger.SpinnerCircle(statusText)
	ct.setSpinner(checkID, spinner)
}

// transitionCheckToNonRunning creates a handle when a check transitions from running state.
func (ct *checkTracker) transitionCheckToNonRunning(logger *bullets.UpdatableLogger, checkID int64, statusText string) {
	// Stop spinner
	if spinner, exists := ct.getSpinner(checkID); exists {
		spinner.Replace(statusText)
		ct.deleteSpinner(checkID)
	}
	// Create static handle
	handle := logger.InfoHandle(statusText)
	ct.setHandle(checkID, handle)
}

// updateExistingCheckDisplay updates existing display without animation state change.
func (ct *checkTracker) updateExistingCheckDisplay(checkID int64, statusText string) {
	if _, exists := ct.getSpinner(checkID); exists {
		// Spinner is running, no update needed (animation continues)
		return
	}
	if handle, exists := ct.getHandle(checkID); exists {
		// Static handle, update text
		handle.Update(bullets.InfoLevel, statusText)
	}
}

// Ensure checkTracker implements StateTracker interface at compile time.
var _ StateTracker = (*checkTracker)(nil)
