package github

import (
	"github.com/sgaunet/bullets"
)

// displayRenderer implements the DisplayRenderer interface using bullets library.
// It provides UI rendering capabilities with proper resource management.
type displayRenderer struct {
	logger         *bullets.Logger
	updatable      *bullets.UpdatableLogger
	activeSpinners map[int64]*bullets.Spinner      // Track active spinners for cleanup
	activeHandles  map[int64]*bullets.BulletHandle // Track active handles for cleanup
}

// newDisplayRenderer creates a new display renderer wrapping bullets library.
func newDisplayRenderer(logger *bullets.Logger, updatable *bullets.UpdatableLogger) *displayRenderer {
	return &displayRenderer{
		logger:         logger,
		updatable:      updatable,
		activeSpinners: make(map[int64]*bullets.Spinner),
		activeHandles:  make(map[int64]*bullets.BulletHandle),
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
	d.activeSpinners = make(map[int64]*bullets.Spinner)
	d.activeHandles = make(map[int64]*bullets.BulletHandle)
}

// Ensure displayRenderer implements DisplayRenderer interface at compile time.
var _ DisplayRenderer = (*displayRenderer)(nil)
