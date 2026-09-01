package github_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
)

// TestWaitForWorkflows_StalledServerHonoursBudget verifies that a server which
// accepts the connection and then never answers cannot outlive the configured
// budget.
//
// This is the failure the timeout was powerless against before the API became
// context-aware: the budget was only checked between polls, so a request that never
// returned blocked the CLI indefinitely no matter what timeout was configured. The
// test cannot even be written without a context-aware client, and it hangs rather
// than fails if the bound regresses.
func TestWaitForWorkflows_StalledServerHonoursBudget(t *testing.T) {
	blocked := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-blocked // Accept the request, then never respond.
	}))
	// Registered before the unblock so it runs after it: Close waits for in-flight
	// handlers, which cannot return until blocked is closed.
	defer srv.Close()
	defer close(blocked)

	t.Setenv("GITHUB_TOKEN", "ghp_stalledservertest")
	client, err := ghpkg.NewClient(t.Context())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := ghpkg.SetBaseURLForTest(client, srv.URL+"/"); err != nil {
		t.Fatalf("SetBaseURLForTest: %v", err)
	}

	const budget = 2 * time.Second
	done := make(chan error, 1)
	start := time.Now()

	go func() {
		_, waitErr := client.WaitForWorkflows(t.Context(), budget)
		done <- waitErr
	}()

	select {
	case waitErr := <-done:
		elapsed := time.Since(start)
		if !errors.Is(waitErr, ghpkg.ErrWorkflowTimeout) {
			t.Errorf("got error %v, want ErrWorkflowTimeout", waitErr)
		}
		// Generous slack: the assertion is "bounded by the budget", not "precise".
		if elapsed > budget+5*time.Second {
			t.Errorf("returned after %v, far beyond the %v budget", elapsed, budget)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("WaitForWorkflows never returned against a stalled server; the wait is unbounded")
	}
}

// TestWaitForWorkflows_ContextCancelled verifies that cancelling the caller's context
// aborts the wait promptly rather than after the current poll interval elapses.
func TestWaitForWorkflows_ContextCancelled(t *testing.T) {
	blocked := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-blocked
	}))
	defer srv.Close()
	defer close(blocked)

	t.Setenv("GITHUB_TOKEN", "ghp_cancelledtest")
	client, err := ghpkg.NewClient(t.Context())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := ghpkg.SetBaseURLForTest(client, srv.URL+"/"); err != nil {
		t.Fatalf("SetBaseURLForTest: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() {
		_, waitErr := client.WaitForWorkflows(ctx, time.Hour)
		done <- waitErr
	}()

	// Let the wait get as far as its first request, then interrupt it.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case waitErr := <-done:
		if !errors.Is(waitErr, ghpkg.ErrWorkflowCanceled) {
			t.Errorf("got error %v, want ErrWorkflowCanceled", waitErr)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("cancelling the context did not stop the wait")
	}
}

// TestWaitForWorkflows_SurvivesTransientRateLimit verifies that a 429 from the
// check-runs endpoint mid-poll is retried rather than ending the run.
//
// Before this behaviour existed, any transient API error aborted auto-mr outright,
// abandoning a pull request whose CI may have been moments from passing and leaving
// the user to start the whole flow again.
//
// The rate limit is keyed on the check-runs path specifically. WaitForWorkflows
// probes other endpoints first and deliberately ignores their errors, so a handler
// that failed the first request it saw regardless of path would have its 429
// swallowed by that probe and never exercise the retry at all.
func TestWaitForWorkflows_SurvivesTransientRateLimit(t *testing.T) {
	var checkRunCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "check-suites"):
			// Report a suite so the wait does not short-circuit as "no CI configured".
			_, _ = w.Write([]byte(`{"total_count":1,"check_suites":[{"id":1}]}`))
			return
		case strings.Contains(r.URL.Path, "/actions/runs"):
			// No Actions runs, so the wait falls through to the check-runs path.
			_, _ = w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
			return
		case !strings.Contains(r.URL.Path, "check-runs"):
			_, _ = w.Write([]byte(`{}`))
			return
		}

		// Rate limit the first check-runs poll, then report a completed success.
		if checkRunCalls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
			return
		}
		_, _ = w.Write([]byte(`{"total_count":1,"check_runs":[` +
			`{"id":1,"name":"build","status":"completed","conclusion":"success"}]}`))
	}))
	defer srv.Close()

	t.Setenv("GITHUB_TOKEN", "ghp_ratelimittest")
	client, err := ghpkg.NewClient(t.Context())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := ghpkg.SetBaseURLForTest(client, srv.URL+"/"); err != nil {
		t.Fatalf("SetBaseURLForTest: %v", err)
	}

	conclusion, err := client.WaitForWorkflows(t.Context(), 30*time.Second)
	if err != nil {
		t.Fatalf("WaitForWorkflows aborted on a transient 429: %v", err)
	}
	if conclusion != "success" {
		t.Errorf("conclusion = %q, want %q", conclusion, "success")
	}
	if got := checkRunCalls.Load(); got < 2 {
		t.Errorf("check-runs endpoint saw %d calls; the 429 was not retried", got)
	}
}
