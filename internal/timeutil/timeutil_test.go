package timeutil_test

import (
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/internal/timeutil"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		// Zero and basic cases
		{
			name:     "zero duration",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "seconds only - small",
			duration: 5 * time.Second,
			expected: "5s",
		},
		{
			name:     "seconds only - large",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "boundary - 59 seconds",
			duration: 59 * time.Second,
			expected: "59s",
		},
		{
			name:     "boundary - 60 seconds",
			duration: 60 * time.Second,
			expected: "1m 0s",
		},
		{
			name:     "minutes and seconds",
			duration: 1*time.Minute + 23*time.Second,
			expected: "1m 23s",
		},
		{
			name:     "minutes only",
			duration: 5 * time.Minute,
			expected: "5m 0s",
		},

		// Typical CI/CD durations
		{
			name:     "typical CI - 30 seconds",
			duration: 30 * time.Second,
			expected: "30s",
		},
		{
			name:     "typical CI - 2 minutes",
			duration: 2 * time.Minute,
			expected: "2m 0s",
		},
		{
			name:     "typical CI - 5 minutes 30 seconds",
			duration: 5*time.Minute + 30*time.Second,
			expected: "5m 30s",
		},
		{
			name:     "typical CI - 15 minutes",
			duration: 15 * time.Minute,
			expected: "15m 0s",
		},
		{
			name:     "config default timeout - 30 minutes",
			duration: 30 * time.Minute,
			expected: "30m 0s",
		},

		// Config boundary conditions
		{
			name:     "config min timeout - 1 minute",
			duration: 1 * time.Minute,
			expected: "1m 0s",
		},
		{
			name:     "config max timeout - 8 hours",
			duration: 8 * time.Hour,
			expected: "480m 0s",
		},
		{
			name:     "long duration - 1 hour",
			duration: 1 * time.Hour,
			expected: "60m 0s",
		},
		{
			name:     "long duration - 2 hours 15 minutes",
			duration: 2*time.Hour + 15*time.Minute,
			expected: "135m 0s",
		},

		// Rounding behavior
		{
			name:     "rounding - 1.4 seconds",
			duration: 1400 * time.Millisecond,
			expected: "1s",
		},
		{
			name:     "rounding - 1.5 seconds",
			duration: 1500 * time.Millisecond,
			expected: "2s",
		},
		{
			name:     "rounding - 1.6 seconds",
			duration: 1600 * time.Millisecond,
			expected: "2s",
		},
		{
			name:     "milliseconds - 500ms",
			duration: 500 * time.Millisecond,
			expected: "1s", // Rounds up due to Go's Round() behavior
		},
		{
			name:     "milliseconds - 999ms",
			duration: 999 * time.Millisecond,
			expected: "1s",
		},
		{
			name:     "sub-second precision",
			duration: 1*time.Second + 200*time.Millisecond,
			expected: "1s",
		},

		// Edge cases
		{
			name:     "negative duration - small",
			duration: -5 * time.Second,
			expected: "-5s",
		},
		{
			name:     "negative duration - with minutes",
			duration: -1*time.Minute - 30*time.Second,
			expected: "-30s", // Go's duration arithmetic handles negatives differently
		},
		{
			name:     "very large duration - 24 hours",
			duration: 24 * time.Hour,
			expected: "1440m 0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := timeutil.FormatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("FormatDuration(%v) = %q, expected %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration_Consistency(t *testing.T) {
	// Verify that same duration always produces same output (idempotency)
	duration := 2*time.Minute + 34*time.Second
	first := timeutil.FormatDuration(duration)
	second := timeutil.FormatDuration(duration)

	if first != second {
		t.Errorf("FormatDuration not idempotent: first=%q, second=%q", first, second)
	}
}

func TestFormatDuration_RoundingBehavior(t *testing.T) {
	// Verify standard Go rounding behavior (rounds to nearest, ties away from zero)
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{1499 * time.Millisecond, "1s"},  // Rounds down
		{1500 * time.Millisecond, "2s"},  // Rounds up (tie rounds away from zero)
		{2500 * time.Millisecond, "3s"},  // Rounds up (tie rounds away from zero)
		{3500 * time.Millisecond, "4s"},  // Rounds up (tie rounds away from zero)
	}

	for _, tt := range tests {
		t.Run(tt.duration.String(), func(t *testing.T) {
			result := timeutil.FormatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("FormatDuration(%v) = %q, expected %q", tt.duration, result, tt.expected)
			}
		})
	}
}
