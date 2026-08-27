package polling_test

import (
	"context"
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/internal/polling"
)

func TestScheduleIntervalFor(t *testing.T) {
	tests := []struct {
		name    string
		elapsed time.Duration
		want    time.Duration
	}{
		// Short pipelines must keep the original cadence; this is the common case and
		// slowing it down would make the tool feel less responsive.
		{name: "start_of_wait", elapsed: 0, want: 5 * time.Second},
		{name: "just_before_growth", elapsed: 59 * time.Second, want: 5 * time.Second},
		{name: "at_growth_boundary", elapsed: 1 * time.Minute, want: 10 * time.Second},
		{name: "between_growth_and_cap", elapsed: 2 * time.Minute, want: 10 * time.Second},
		{name: "at_cap_boundary", elapsed: 3 * time.Minute, want: 20 * time.Second},
		{name: "long_wait_is_capped", elapsed: 8 * time.Hour, want: 20 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := polling.DefaultSchedule.IntervalFor(tt.elapsed); got != tt.want {
				t.Errorf("IntervalFor(%v) = %v, want %v", tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestSleepCompletes(t *testing.T) {
	if !polling.Sleep(t.Context(), time.Millisecond) {
		t.Error("Sleep reported interruption for an uncancelled context")
	}
}

func TestSleepInterruptedByContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	start := time.Now()
	if polling.Sleep(ctx, time.Hour) {
		t.Error("Sleep reported completion for a cancelled context")
	}
	// The point of Sleep is that cancellation does not wait out the interval.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Sleep took %v to observe cancellation", elapsed)
	}
}
