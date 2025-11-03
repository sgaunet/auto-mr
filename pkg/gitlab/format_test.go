package gitlab

import (
	"strings"
	"testing"
	"time"
)

// TestFormatJobStatus tests the formatJobStatus function with various job states.
func TestFormatJobStatus(t *testing.T) {
	tests := []struct {
		name     string
		job      *Job
		expected []string // Expected substrings in the output
	}{
		{
			name: "nil job",
			job:  nil,
			expected: []string{""},
		},
		{
			name: "running job with duration",
			job: &Job{
				ID:       1,
				Name:     "build",
				Stage:    "test",
				Status:   statusRunning,
				Duration: 83.5,
			},
			expected: []string{iconRunning, "test/build", "running", "1m 23s"},
		},
		{
			name: "success job",
			job: &Job{
				ID:       2,
				Name:     "test",
				Stage:    "test",
				Status:   statusSuccess,
				Duration: 45.0,
			},
			expected: []string{iconSuccess, "test/test", "success", "45s"},
		},
		{
			name: "failed job",
			job: &Job{
				ID:       3,
				Name:     "deploy",
				Stage:    "deploy",
				Status:   statusFailed,
				Duration: 10.0,
			},
			expected: []string{iconFailed, "deploy/deploy", "failed", "10s"},
		},
		{
			name: "canceled job",
			job: &Job{
				ID:     4,
				Name:   "cleanup",
				Stage:  "cleanup",
				Status: statusCanceled,
			},
			expected: []string{iconCanceled, "cleanup/cleanup", "canceled"},
		},
		{
			name: "skipped job",
			job: &Job{
				ID:     5,
				Name:   "optional",
				Status: statusSkipped,
			},
			expected: []string{iconSkipped, "optional", "skipped"},
		},
		{
			name: "pending job without stage",
			job: &Job{
				ID:     6,
				Name:   "waiting",
				Status: statusPending,
			},
			expected: []string{iconPending, "waiting", "pending"},
		},
		{
			name: "created job",
			job: &Job{
				ID:     7,
				Name:   "init",
				Status: statusCreated,
			},
			expected: []string{iconPending, "init", "created"},
		},
		{
			name: "running job with elapsed time",
			job: func() *Job {
				startTime := time.Now().Add(-2 * time.Minute)
				return &Job{
					ID:        8,
					Name:      "long-runner",
					Status:    statusRunning,
					StartedAt: &startTime,
				}
			}(),
			expected: []string{iconRunning, "long-runner", "running", "2m"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatJobStatus(tt.job)

			// Handle empty case
			if len(tt.expected) == 1 && tt.expected[0] == "" {
				if result != "" {
					t.Errorf("Expected empty string, got %q", result)
				}
				return
			}

			// Check all expected substrings are present
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("Expected %q to contain %q, got %q", result, exp, result)
				}
			}
		})
	}
}

// TestFormatJobStatusIconMapping tests that correct icons are used for each status.
func TestFormatJobStatusIconMapping(t *testing.T) {
	tests := []struct {
		status       string
		expectedIcon string
	}{
		{statusRunning, iconRunning},
		{statusSuccess, iconSuccess},
		{statusFailed, iconFailed},
		{statusCanceled, iconCanceled},
		{statusSkipped, iconSkipped},
		{statusPending, iconPending},
		{statusCreated, iconPending},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			job := &Job{
				ID:     1,
				Name:   "test",
				Status: tt.status,
			}
			result := formatJobStatus(job)
			if !strings.HasPrefix(result, tt.expectedIcon) {
				t.Errorf("Expected icon %q for status %q, result: %q", tt.expectedIcon, tt.status, result)
			}
		})
	}
}

// TestFormatJobStatusEdgeCases tests edge cases.
func TestFormatJobStatusEdgeCases(t *testing.T) {
	t.Run("empty job name", func(t *testing.T) {
		job := &Job{
			ID:     1,
			Name:   "",
			Status: statusSuccess,
		}
		result := formatJobStatus(job)
		if result == "" {
			t.Error("Should handle empty job name")
		}
	})

	t.Run("empty stage", func(t *testing.T) {
		job := &Job{
			ID:     1,
			Name:   "test",
			Stage:  "",
			Status: statusSuccess,
		}
		result := formatJobStatus(job)
		// Should not have stage prefix with "/"
		if strings.Contains(result, "/") {
			t.Errorf("Should not include stage separator when stage is empty: %q", result)
		}
	})

	t.Run("unknown status", func(t *testing.T) {
		job := &Job{
			ID:     1,
			Name:   "test",
			Status: "unknown",
		}
		result := formatJobStatus(job)
		// Should still return something
		if result == "" {
			t.Error("Should handle unknown status gracefully")
		}
		// Should default to pending icon
		if !strings.HasPrefix(result, iconPending) {
			t.Errorf("Unknown status should default to pending icon, got: %q", result)
		}
	})
}

// TestFormatDuration tests the formatDuration helper function.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "seconds only",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "one minute",
			duration: 60 * time.Second,
			expected: "1m 0s",
		},
		{
			name:     "minutes and seconds",
			duration: 83 * time.Second,
			expected: "1m 23s",
		},
		{
			name:     "multiple minutes",
			duration: 5*time.Minute + 30*time.Second,
			expected: "5m 30s",
		},
		{
			name:     "hours converted to minutes",
			duration: 2*time.Hour + 15*time.Minute + 30*time.Second,
			expected: "135m 30s",
		},
		{
			name:     "subsecond rounded",
			duration: 1500 * time.Millisecond,
			expected: "2s", // Rounded up
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}
