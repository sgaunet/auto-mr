package github

import (
	"strings"
	"testing"
	"time"
)

// TestFormatJobStatus tests the formatJobStatus function with various job states.
func TestFormatJobStatus(t *testing.T) {
	now := time.Now()
	twoMinutesAgo := now.Add(-2 * time.Minute)
	threeMinutesAgo := now.Add(-3 * time.Minute)

	tests := []struct {
		name     string
		job      *JobInfo
		expected []string // Expected substrings in the output
	}{
		{
			name: "nil job",
			job:  nil,
			expected: []string{""},
		},
		{
			name: "queued job",
			job: &JobInfo{
				ID:     1,
				Name:   "build",
				Status: statusQueued,
			},
			expected: []string{iconPending, "build", "queued"},
		},
		{
			name: "in_progress job with elapsed time",
			job: &JobInfo{
				ID:        2,
				Name:      "test",
				Status:    statusInProgress,
				StartedAt: &twoMinutesAgo,
			},
			expected: []string{iconRunning, "test", "running", "2m"},
		},
		{
			name: "completed success job",
			job: &JobInfo{
				ID:          3,
				Name:        "deploy",
				Status:      statusCompleted,
				Conclusion:  conclusionSuccess,
				StartedAt:   &threeMinutesAgo,
				CompletedAt: &twoMinutesAgo,
			},
			expected: []string{iconSuccess, "deploy", "success", "1m"},
		},
		{
			name: "completed failed job",
			job: &JobInfo{
				ID:          4,
				Name:        "integration-test",
				Status:      statusCompleted,
				Conclusion:  "failure",
				StartedAt:   &threeMinutesAgo,
				CompletedAt: &now,
			},
			expected: []string{iconFailed, "integration-test", "failure", "3m"},
		},
		{
			name: "completed cancelled job",
			job: &JobInfo{
				ID:         5,
				Name:       "cleanup",
				Status:     statusCompleted,
				Conclusion: "cancelled",
			},
			expected: []string{iconCanceled, "cleanup", "cancelled"},
		},
		{
			name: "completed skipped job",
			job: &JobInfo{
				ID:         6,
				Name:       "optional-check",
				Status:     statusCompleted,
				Conclusion: conclusionSkipped,
			},
			expected: []string{iconSkipped, "optional-check", "skipped"},
		},
		{
			name: "completed neutral job",
			job: &JobInfo{
				ID:         7,
				Name:       "info-check",
				Status:     statusCompleted,
				Conclusion: conclusionNeutral,
			},
			expected: []string{iconSkipped, "info-check", "neutral"},
		},
		{
			name: "completed timed_out job",
			job: &JobInfo{
				ID:         8,
				Name:       "long-running",
				Status:     statusCompleted,
				Conclusion: "timed_out",
			},
			expected: []string{iconFailed, "long-running", "timed_out"},
		},
		{
			name: "in_progress without start time",
			job: &JobInfo{
				ID:     9,
				Name:   "starting",
				Status: statusInProgress,
			},
			expected: []string{iconRunning, "starting", "running"},
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

// TestFormatJobStatusIconMapping tests that correct icons are used for each status/conclusion.
func TestFormatJobStatusIconMapping(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		conclusion   string
		expectedIcon string
	}{
		{"queued", statusQueued, "", iconPending},
		{"in_progress", statusInProgress, "", iconRunning},
		{"completed success", statusCompleted, conclusionSuccess, iconSuccess},
		{"completed failure", statusCompleted, "failure", iconFailed},
		{"completed cancelled", statusCompleted, "cancelled", iconCanceled},
		{"completed skipped", statusCompleted, conclusionSkipped, iconSkipped},
		{"completed neutral", statusCompleted, conclusionNeutral, iconSkipped},
		{"completed timed_out", statusCompleted, "timed_out", iconFailed},
		{"completed action_required", statusCompleted, "action_required", iconFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &JobInfo{
				ID:         1,
				Name:       "test",
				Status:     tt.status,
				Conclusion: tt.conclusion,
			}
			result := formatJobStatus(job)
			if !strings.HasPrefix(result, tt.expectedIcon) {
				t.Errorf("Expected icon %q for %s/%s, result: %q", tt.expectedIcon, tt.status, tt.conclusion, result)
			}
		})
	}
}

// TestFormatJobStatusEdgeCases tests edge cases.
func TestFormatJobStatusEdgeCases(t *testing.T) {
	t.Run("empty job name", func(t *testing.T) {
		job := &JobInfo{
			ID:         1,
			Name:       "",
			Status:     statusCompleted,
			Conclusion: conclusionSuccess,
		}
		result := formatJobStatus(job)
		if result == "" {
			t.Error("Should handle empty job name")
		}
	})

	t.Run("unknown status", func(t *testing.T) {
		job := &JobInfo{
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

	t.Run("completed with empty conclusion", func(t *testing.T) {
		job := &JobInfo{
			ID:         1,
			Name:       "test",
			Status:     statusCompleted,
			Conclusion: "",
		}
		result := formatJobStatus(job)
		// Should still format properly
		if result == "" {
			t.Error("Should handle empty conclusion")
		}
	})

	t.Run("nil timestamps", func(t *testing.T) {
		job := &JobInfo{
			ID:          1,
			Name:        "test",
			Status:      statusCompleted,
			Conclusion:  conclusionSuccess,
			StartedAt:   nil,
			CompletedAt: nil,
		}
		result := formatJobStatus(job)
		// Should not include duration when timestamps are nil
		// Check for time patterns like "1m 23s" or "45s"
		if strings.Contains(result, "m ") || strings.Contains(result, "s)") && !strings.HasSuffix(result, "(success)") {
			t.Errorf("Should not include duration with nil timestamps: %q", result)
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

// TestFormatJobStatusDurationCalculation tests duration calculation logic.
func TestFormatJobStatusDurationCalculation(t *testing.T) {
	t.Run("completed job with both timestamps", func(t *testing.T) {
		start := time.Now().Add(-2 * time.Minute)
		end := time.Now()
		job := &JobInfo{
			ID:          1,
			Name:        "test",
			Status:      statusCompleted,
			Conclusion:  conclusionSuccess,
			StartedAt:   &start,
			CompletedAt: &end,
		}
		result := formatJobStatus(job)
		// Should have roughly 2 minutes
		if !strings.Contains(result, "2m") && !strings.Contains(result, "1m 5") {
			t.Errorf("Expected ~2m duration, got: %q", result)
		}
	})

	t.Run("in_progress job with start time", func(t *testing.T) {
		start := time.Now().Add(-30 * time.Second)
		job := &JobInfo{
			ID:        1,
			Name:      "test",
			Status:    statusInProgress,
			StartedAt: &start,
		}
		result := formatJobStatus(job)
		// Should have roughly 30 seconds
		if !strings.Contains(result, "s") {
			t.Errorf("Expected duration with seconds, got: %q", result)
		}
	})

	t.Run("completed job with only start timestamp", func(t *testing.T) {
		start := time.Now().Add(-1 * time.Minute)
		job := &JobInfo{
			ID:          1,
			Name:        "test",
			Status:      statusCompleted,
			Conclusion:  conclusionSuccess,
			StartedAt:   &start,
			CompletedAt: nil, // Missing end time
		}
		result := formatJobStatus(job)
		// Should not calculate duration without CompletedAt
		// Check for time patterns like "1m 23s" or "45s"
		hasDuration := strings.Contains(result, "m ") || (strings.Contains(result, "s)") && !strings.HasSuffix(result, "(success)"))
		if hasDuration {
			t.Errorf("Should not include duration with missing CompletedAt: %q", result)
		}
	})
}
