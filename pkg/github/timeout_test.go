package github_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
