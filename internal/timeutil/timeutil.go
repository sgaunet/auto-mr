// Package timeutil provides time formatting utilities for human-readable duration display.
//
// Durations are formatted as "Xm Ys" for durations of one minute or more,
// and "Ys" for shorter durations. The value is rounded to the nearest second.
//
// Examples:
//
//	timeutil.FormatDuration(83 * time.Second)  // "1m 23s"
//	timeutil.FormatDuration(45 * time.Second)  // "45s"
//	timeutil.FormatDuration(8 * time.Hour)     // "480m 0s"
package timeutil

import (
	"fmt"
	"time"
)

// FormatDuration formats a duration into a human-readable string.
// It rounds to the nearest second and displays in "Xm Ys" or "Ys" format.
//
// Examples:
//   - 1m 23s for durations >= 1 minute
//   - 45s for durations < 1 minute
//   - 480m 0s for 8-hour duration (no hour formatting)
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	minutes := d / time.Minute
	seconds := (d % time.Minute) / time.Second

	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
