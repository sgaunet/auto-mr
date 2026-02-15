package gitlab

import (
	"fmt"
	"time"

	"github.com/sgaunet/auto-mr/internal/timeutil"
	"github.com/sgaunet/bullets"
)

// displayRenderer implements display operations using bullets library.
// It provides UI rendering capabilities with proper resource management.
type displayRenderer struct {
	logger         *bullets.Logger
	updatable      *bullets.UpdatableLogger
	activeSpinners map[int]*bullets.Spinner      // Track active spinners for cleanup
	activeHandles  map[int]*bullets.BulletHandle // Track active handles for cleanup
}

// newDisplayRenderer creates a new display renderer wrapping bullets library.
func newDisplayRenderer(logger *bullets.Logger, updatable *bullets.UpdatableLogger) *displayRenderer {
	return &displayRenderer{
		logger:         logger,
		updatable:      updatable,
		activeSpinners: make(map[int]*bullets.Spinner),
		activeHandles:  make(map[int]*bullets.BulletHandle),
	}
}

// SetLogger updates the logger used by the display renderer.
// This updates both the debug logger and the updatable logger.
func (d *displayRenderer) SetLogger(logger *bullets.Logger) {
	d.logger = logger
	d.updatable.Logger = logger
}

// GetUpdatable returns the underlying updatable logger.
// This is used internally for operations that need direct access to the updatable logger,
// such as tracker updates.
func (d *displayRenderer) GetUpdatable() *bullets.UpdatableLogger {
	return d.updatable
}

// Info logs an informational message.
func (d *displayRenderer) Info(message string) {
	d.updatable.Info(message)
}

// Debug logs a debug message.
func (d *displayRenderer) Debug(message string) {
	d.logger.Debug(message)
}

// Error logs an error message.
func (d *displayRenderer) Error(message string) {
	d.updatable.Error(message)
}

// Success logs a success message.
func (d *displayRenderer) Success(message string) {
	d.updatable.Success(message)
}

// InfoHandle creates an updatable handle for an info message.
// The handle can be updated with new content or converted to success/error.
func (d *displayRenderer) InfoHandle(message string) *bullets.BulletHandle {
	handle := d.updatable.InfoHandle(message)
	return handle
}

// SpinnerCircle creates an animated spinner with the given message.
// Returns a Spinner that can be stopped with Success(), Error(), or Replace().
func (d *displayRenderer) SpinnerCircle(message string) *bullets.Spinner {
	spinner := d.updatable.SpinnerCircle(message)
	return spinner
}

// IncreasePadding increases the indentation level for nested output.
func (d *displayRenderer) IncreasePadding() {
	d.updatable.IncreasePadding()
}

// DecreasePadding decreases the indentation level for nested output.
func (d *displayRenderer) DecreasePadding() {
	d.updatable.DecreasePadding()
}

// Cleanup stops all active spinners and clears handles.
// This should be called when the display is no longer needed.
func (d *displayRenderer) Cleanup() {
	for _, spinner := range d.activeSpinners {
		if spinner != nil {
			spinner.Stop()
		}
	}
	d.activeSpinners = make(map[int]*bullets.Spinner)
	d.activeHandles = make(map[int]*bullets.BulletHandle)
}

// formatJobStatus formats a job status with duration.
// Returns a formatted string like "build (running, 1m 23s)" or "test (success, 45s)".
// Icons are added by the bullets library methods (Success/Error/etc), not by this function.
func formatJobStatus(job *Job) string {
	if job == nil {
		return ""
	}

	// Build job name with stage prefix if available
	jobName := job.Name
	if job.Stage != "" {
		jobName = job.Stage + "/" + job.Name
	}

	// Calculate duration
	var durationStr string
	if job.Duration > 0 {
		durationStr = timeutil.FormatDuration(time.Duration(job.Duration) * time.Second)
	} else if job.StartedAt != nil && job.Status == statusRunning {
		// Calculate elapsed time for running jobs
		elapsed := time.Since(*job.StartedAt)
		durationStr = timeutil.FormatDuration(elapsed)
	}

	// Format the complete status string (without icon - bullets library adds those)
	if durationStr != "" {
		return fmt.Sprintf("%s (%s, %s)", jobName, job.Status, durationStr)
	}
	return fmt.Sprintf("%s (%s)", jobName, job.Status)
}
