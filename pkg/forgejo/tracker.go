package forgejo

import (
	"context"
	"fmt"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/sgaunet/bullets"
)

// newStatusTracker creates a new status tracker with initialized maps.
// The tracker owns a context derived from statusCtx, which scopes both the spinner
// animations it creates and its own refresh goroutines. Callers must call [Stop] when
// the wait ends, including on error: a status left in a pending state would otherwise
// keep its goroutine ticking, since those loops exit only when the status data
// reaches a terminal state.
func newStatusTracker(ctx context.Context) *statusTracker {
	ctx, cancel := context.WithCancel(ctx)
	return &statusTracker{
		ctx:      ctx,
		cancel:   cancel,
		entries:  make(map[string]*statusEntry),
		handles:  make(map[string]*bullets.BulletHandle),
		spinners: make(map[string]*bullets.Spinner),
	}
}

// Stop tears down the tracker's spinners and refresh goroutines.
func (st *statusTracker) Stop() {
	st.cancel()
}

// getEntry retrieves a status entry by context name with read lock.
func (st *statusTracker) getEntry(statusCtx string) (*statusEntry, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	entry, exists := st.entries[statusCtx]
	return entry, exists
}

// setEntry stores a status entry by context name with write lock.
func (st *statusTracker) setEntry(statusCtx string, entry *statusEntry) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.entries[statusCtx] = entry
}

// getHandle retrieves a bullet handle by context name with read lock.
func (st *statusTracker) getHandle(statusCtx string) (*bullets.BulletHandle, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	handle, exists := st.handles[statusCtx]
	return handle, exists
}

// setHandle stores a bullet handle for a context name with write lock.
func (st *statusTracker) setHandle(statusCtx string, handle *bullets.BulletHandle) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.handles[statusCtx] = handle
}

// getSpinner retrieves a spinner by context name with read lock.
func (st *statusTracker) getSpinner(statusCtx string) (*bullets.Spinner, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	spinner, exists := st.spinners[statusCtx]
	return spinner, exists
}

// setSpinner stores a spinner for a context name with write lock.
func (st *statusTracker) setSpinner(statusCtx string, spinner *bullets.Spinner) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.spinners[statusCtx] = spinner
}

// deleteSpinner removes a spinner with write lock, stopping its animation first.
func (st *statusTracker) deleteSpinner(statusCtx string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if spinner, exists := st.spinners[statusCtx]; exists {
		spinner.Stop()
		delete(st.spinners, statusCtx)
	}
}

// update processes new commit statuses, creates/updates display handles, and returns
// a list of state transition descriptions for debug logging.
func (st *statusTracker) update(statuses []*gitea.Status, logger *bullets.UpdatableLogger) []string {
	var transitions []string

	for _, s := range statuses {
		if s == nil || s.Context == "" {
			continue
		}

		entry := &statusEntry{
			context:     s.Context,
			state:       s.State,
			targetURL:   s.TargetURL,
			description: s.Description,
		}

		transition := st.processStatusUpdate(entry, logger)
		if transition != "" {
			transitions = append(transitions, transition)
		}
	}

	return transitions
}

// processStatusUpdate handles the update logic for a single status entry.
func (st *statusTracker) processStatusUpdate(newEntry *statusEntry, logger *bullets.UpdatableLogger) string {
	oldEntry, exists := st.getEntry(newEntry.context)

	if !exists {
		return st.handleNewStatus(newEntry, logger)
	}

	if oldEntry.state != newEntry.state {
		return st.handleStatusChange(oldEntry, newEntry, logger)
	}

	// No state change – update the stored description in case it changed.
	st.setEntry(newEntry.context, newEntry)
	return ""
}

// handleNewStatus processes a newly detected commit status context.
func (st *statusTracker) handleNewStatus(entry *statusEntry, logger *bullets.UpdatableLogger) string {
	st.setEntry(entry.context, entry)
	label := formatStatusLabel(entry)

	if entry.state == gitea.StatusPending {
		spinner := logger.SpinnerCircle(st.ctx, label)
		st.setSpinner(entry.context, spinner)

		go st.updateSpinnerLoop(entry.context, spinner)
	} else {
		handle := logger.InfoHandle(label)
		st.setHandle(entry.context, handle)
		st.finalizeHandle(entry.context, entry.state, label)
	}

	return fmt.Sprintf("status %s: new state %s", entry.context, entry.state)
}

// handleStatusChange processes a commit status context that transitioned state.
func (st *statusTracker) handleStatusChange(
	oldEntry, newEntry *statusEntry,
	logger *bullets.UpdatableLogger,
) string {
	st.setEntry(newEntry.context, newEntry)
	label := formatStatusLabel(newEntry)

	wasPending := oldEntry.state == gitea.StatusPending
	isPending := newEntry.state == gitea.StatusPending

	switch {
	case isPending && !wasPending:
		// Transitioned to pending – create a spinner.
		spinner := logger.SpinnerCircle(st.ctx, label)
		st.setSpinner(newEntry.context, spinner)
		go st.updateSpinnerLoop(newEntry.context, spinner)

	case !isPending && wasPending:
		// Was pending, now resolved – stop spinner and finalize.
		if spinner, exists := st.getSpinner(newEntry.context); exists {
			st.finalizeSpinner(spinner, newEntry.state, label)
			st.deleteSpinner(newEntry.context)
		}

	default:
		// Was already resolved; update the static handle if present.
		if handle, exists := st.getHandle(newEntry.context); exists {
			handle.Update(bullets.InfoLevel, label)
			st.finalizeHandle(newEntry.context, newEntry.state, label)
		}
	}

	return fmt.Sprintf("status %s: %s -> %s", newEntry.context, oldEntry.state, newEntry.state)
}

// finalizeSpinner stops a spinner with the appropriate symbol.
func (st *statusTracker) finalizeSpinner(spinner *bullets.Spinner, state gitea.StatusState, label string) {
	switch state {
	case gitea.StatusSuccess, gitea.StatusWarning:
		spinner.Success(label)
	case gitea.StatusPending:
		// Pending means we are still waiting; just update the text without finalizing.
		spinner.UpdateText(label)
	case gitea.StatusFailure, gitea.StatusError:
		spinner.Error(label)
	}
}

// finalizeHandle updates an existing handle with the terminal state symbol.
func (st *statusTracker) finalizeHandle(statusCtx string, state gitea.StatusState, label string) {
	handle, exists := st.getHandle(statusCtx)
	if !exists {
		return
	}

	switch state {
	case gitea.StatusSuccess, gitea.StatusWarning:
		handle.Success(label)
	case gitea.StatusPending:
		handle.Update(bullets.InfoLevel, label)
	case gitea.StatusFailure, gitea.StatusError:
		handle.Error(label)
	}
}

// updateSpinnerLoop continuously refreshes spinner text while the status is pending.
// Runs in a background goroutine. Terminates when the context resolves.
func (st *statusTracker) updateSpinnerLoop(statusCtx string, spinner *bullets.Spinner) {
	ticker := time.NewTicker(spinnerUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
		}

		entry, exists := st.getEntry(statusCtx)
		if !exists {
			break
		}

		if entry.state != gitea.StatusPending {
			break
		}

		if _, spinnerExists := st.getSpinner(statusCtx); !spinnerExists {
			break
		}

		spinner.UpdateText(formatStatusLabel(entry))
	}
}

// formatStatusLabel returns a human-readable label for a status entry.
func formatStatusLabel(entry *statusEntry) string {
	if entry.description != "" {
		return fmt.Sprintf("%s (%s)", entry.context, entry.description)
	}
	return fmt.Sprintf("%s (%s)", entry.context, entry.state)
}
