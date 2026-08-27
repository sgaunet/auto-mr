// Package polling provides the timing primitives shared by the GitLab, GitHub and
// Forgejo CI pollers: a bound for a single API request, and a wait that a context
// can interrupt.
//
// The three platform packages each poll a remote CI system until it reaches a
// terminal state. They need the same two guarantees, and getting either wrong is
// what lets the CLI hang: every individual request must be bounded so a stalled
// response cannot consume the whole budget, and every wait between requests must be
// abandonable so cancellation takes effect immediately rather than after the
// interval elapses.
package polling

import (
	"context"
	"net/http"
	"time"
)

// PerCallTimeout bounds a single API request made while polling.
//
// It is deliberately much shorter than any pipeline budget: a request that has not
// answered within this window is treated as stalled and retried on the next poll,
// which costs one interval, whereas waiting on it indefinitely costs the entire run.
const PerCallTimeout = 20 * time.Second

// OperationTimeout bounds a one-shot API operation such as creating or merging a
// request.
//
// It is more generous than [PerCallTimeout] because several of these operations issue
// a handful of sequential requests — creating a pull request also resolves users,
// assignees and labels — and the bound covers the operation as a whole rather than
// each request within it. Without such a bound a caller whose own context has no
// deadline can hang indefinitely: a merge that stalls after CI has already passed
// leaves the request in limbo one call short of done.
const OperationTimeout = 60 * time.Second

// Sleep waits for d, or until ctx is done, whichever comes first. It reports whether
// the full duration elapsed; false means ctx ended and the caller should stop.
//
// A plain time.Sleep between polls would keep the process alive for up to a full
// interval after cancellation, so an interrupt during a long CI wait would appear to
// be ignored.
func Sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Schedule describes how the interval between polls grows as a wait lengthens.
//
// A long CI run does not need to be checked as eagerly as a short one. Holding a
// flat interval for hours multiplies request volume for no benefit -- an eight-hour
// wait at five seconds is nearly six thousand requests -- while shortening the
// interval for quick pipelines is what keeps the common case responsive. The
// progression is a step function rather than a smooth curve because it is far easier
// to reason about and to test, and the difference is immaterial at these scales.
type Schedule struct {
	// Base applies until GrowAfter has elapsed, Grown until CapAfter, then Cap.
	Base, Grown, Cap    time.Duration
	GrowAfter, CapAfter time.Duration
}

// IntervalFor returns the interval to wait next, given how long the wait has already
// been running.
func (s Schedule) IntervalFor(elapsed time.Duration) time.Duration {
	switch {
	case elapsed < s.GrowAfter:
		return s.Base
	case elapsed < s.CapAfter:
		return s.Grown
	default:
		return s.Cap
	}
}

// DefaultSchedule is the cadence used by the CI pollers.
//
// Pipelines that finish inside a minute see the same five-second cadence as before,
// so nothing about the common case changes; only long waits slow their polling down.
var DefaultSchedule = Schedule{
	Base:      baseInterval,
	Grown:     grownInterval,
	Cap:       cappedInterval,
	GrowAfter: growAfter,
	CapAfter:  capAfter,
}

// Cadence for [DefaultSchedule]. Named rather than inline so the progression reads
// as one policy: poll briskly at first, then ease off as a wait proves to be long.
const (
	baseInterval   = 5 * time.Second
	grownInterval  = 10 * time.Second
	cappedInterval = 20 * time.Second
	growAfter      = 1 * time.Minute
	capAfter       = 3 * time.Minute
)

// Transient reports whether an HTTP status code names a condition worth retrying:
// the server asked us to slow down, or it failed in a way that is usually temporary.
//
// A transient failure mid-poll must not end the run. Aborting on the first blip
// abandons a merge request whose CI may well have been about to pass, and leaves the
// user to start over.
func Transient(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}
