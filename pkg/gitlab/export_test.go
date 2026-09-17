package gitlab

import "github.com/sgaunet/auto-mr/internal/polling"

// SetMergeabilityScheduleForTest shortens the mergeability poll cadence.
//
// The production cadence is right for a real GitLab but would make multi-round wait
// tests dominate the suite's runtime -- the same reason TestMain shortens
// [polling.DefaultSchedule]. The intervals themselves are covered in
// internal/polling; here only the loop behaviour matters.
func SetMergeabilityScheduleForTest(s polling.Schedule) {
	mergeabilitySchedule = s
}
