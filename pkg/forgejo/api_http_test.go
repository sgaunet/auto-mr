package forgejo_test

import (
	"encoding/json"
	"errors"
	"github.com/sgaunet/auto-mr/internal/polling"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/pkg/forgejo"
)

// These tests drive the real *forgejo.Client against a stub Forgejo API.
//
// Note that gitea.NewClient probes GET /api/v1/version before returning, so every
// stub must serve it or construction fails before the test body runs. That probe is
// also why the previous "real client" test always skipped: it pointed at a reserved
// example domain, could never reach it, and skipped itself on the resulting error.

const stubVersion = `{"version":"1.21.0"}`

// forgejoStub builds a server that answers the version probe and delegates
// everything else to handler.
func forgejoStub(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/version") {
			_, _ = w.Write([]byte(stubVersion))
			return
		}
		handler(w, r)
	}))
}

func newTestClient(t *testing.T, srv *httptest.Server) *forgejo.Client {
	t.Helper()

	t.Setenv("FORGEJO_TOKEN", "forgejo-httptestclient12345")
	client, err := forgejo.NewClient(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

// setRepo populates the owner and repository the SDK needs to build request paths.
func setRepo(t *testing.T, client *forgejo.Client) {
	t.Helper()

	if err := client.SetRepositoryFromURL(t.Context(), "https://forgejo.example.com/owner/repo.git"); err != nil {
		t.Fatalf("SetRepositoryFromURL: %v", err)
	}
}

// TestNewClient_ReachesServer replaces a test that always skipped. Pointing at a
// real server means construction, including the SDK's version probe, is actually
// exercised.
func TestNewClient_ReachesServer(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{})
	})
	defer srv.Close()

	t.Setenv("FORGEJO_TOKEN", "forgejo-httptestclient12345")
	client, err := forgejo.NewClient(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("NewClient against a reachable server: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned a nil client without an error")
	}
}

func TestSetRepositoryFromURL(t *testing.T) {
	var requested atomic.Value

	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		requested.Store(r.URL.Path)
		writeJSON(t, w, map[string]any{"name": "repo", "full_name": "owner/repo"})
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	if err := client.SetRepositoryFromURL(t.Context(), "git@forgejo.example.com:owner/repo.git"); err != nil {
		t.Fatalf("SetRepositoryFromURL: %v", err)
	}

	path, _ := requested.Load().(string)
	if !strings.Contains(path, "owner") || !strings.Contains(path, "repo") {
		t.Errorf("requested %q, which does not name the repository from the remote", path)
	}
}

func TestListLabels_ReturnsNames(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/labels") {
			writeJSON(t, w, []map[string]any{
				{"id": 1, "name": "bug"},
				{"id": 2, "name": "enhancement"},
			})
			return
		}
		writeJSON(t, w, map[string]any{})
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	setRepo(t, client)

	labels, err := client.ListLabels(t.Context())
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 2 || labels[0].Name != "bug" {
		t.Errorf("labels = %+v", labels)
	}
}

func TestCreatePullRequest_Success(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/labels"):
			writeJSON(t, w, []map[string]any{{"id": 1, "name": "bug"}})
		case strings.HasSuffix(r.URL.Path, "/pulls") && r.Method == http.MethodPost:
			writeJSON(t, w, map[string]any{
				"number":   42,
				"title":    "Add a feature",
				"html_url": "https://forgejo.example.com/o/r/pulls/42",
				"head":     map[string]any{"ref": "feature", "sha": "deadbeef"},
				"base":     map[string]any{"ref": "main"},
			})
		default:
			writeJSON(t, w, map[string]any{})
		}
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	setRepo(t, client)

	pr, err := client.CreatePullRequest(t.Context(),
		"feature", "main", "Add a feature", "body", "alice", "bob", []string{"bug"})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.Index != 42 {
		t.Errorf("Index = %d, want 42", pr.Index)
	}
}

// TestCreatePullRequest_AlreadyExists covers the 409 classification the adapter maps
// onto platform.ErrAlreadyExists.
func TestCreatePullRequest_AlreadyExists(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			writeJSON(t, w, map[string]any{"message": "pull request already exists"})
			return
		}
		writeJSON(t, w, map[string]any{})
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	setRepo(t, client)

	_, err := client.CreatePullRequest(t.Context(),
		"feature", "main", "t", "b", "alice", "bob", nil)

	if !errors.Is(err, forgejo.ErrPRAlreadyExists) {
		t.Errorf("got %v, want it to wrap ErrPRAlreadyExists", err)
	}
}

func TestGetPullRequestByBranch_NotFound(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls") {
			writeJSON(t, w, []map[string]any{}) // No open pull requests.
			return
		}
		writeJSON(t, w, map[string]any{})
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	setRepo(t, client)

	_, err := client.GetPullRequestByBranch(t.Context(), "feature", "main")

	if !errors.Is(err, forgejo.ErrPRNotFound) {
		t.Errorf("got %v, want ErrPRNotFound", err)
	}
}

func TestMergePullRequest_SurfacesFailure(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/merge") {
			w.WriteHeader(http.StatusMethodNotAllowed)
			writeJSON(t, w, map[string]any{"message": "not mergeable"})
			return
		}
		writeJSON(t, w, map[string]any{})
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	setRepo(t, client)

	if err := client.MergePullRequest(t.Context(), 42, true, "Add a feature"); err == nil {
		t.Error("expected an error for a rejected merge")
	}
}

// statusStub serves a combined status, switching to the terminal state after the
// given number of polls so the wait exercises more than one round.
func statusStub(t *testing.T, terminalState string, pollsBeforeTerminal int32) *httptest.Server {
	t.Helper()

	var polls atomic.Int32
	return forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls") && r.Method == http.MethodPost {
			writeJSON(t, w, map[string]any{
				"number": 42,
				"head":   map[string]any{"ref": "feature", "sha": "deadbeef"},
				"base":   map[string]any{"ref": "main"},
			})
			return
		}
		if !strings.Contains(r.URL.Path, "/status") {
			writeJSON(t, w, map[string]any{"name": "repo", "full_name": "owner/repo"})
			return
		}

		state := "pending"
		if polls.Add(1) > pollsBeforeTerminal {
			state = terminalState
		}
		writeJSON(t, w, map[string]any{
			"state":       state,
			"sha":         "deadbeef",
			"total_count": 1,
			"statuses": []map[string]any{
				{"context": "ci/build", "state": state, "description": "build"},
			},
		})
	})
}

// prepareForWait puts the client into the state a pipeline wait requires: the
// repository, and the pull request whose commit statuses are polled.
func prepareForWait(t *testing.T, client *forgejo.Client) {
	t.Helper()

	setRepo(t, client)
	if _, err := client.CreatePullRequest(t.Context(),
		"feature", "main", "t", "b", "alice", "bob", nil); err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
}

func TestWaitForPipeline_Outcomes(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		wantStatus string
	}{
		{name: "success", state: "success", wantStatus: "success"},
		{name: "failure", state: "failure", wantStatus: "failure"},
		{name: "error", state: "error", wantStatus: "error"},
		// A warning is treated as a pass so a non-blocking check cannot stall a merge.
		{name: "warning_counts_as_success", state: "warning", wantStatus: "success"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := statusStub(t, tt.state, 0)
			defer srv.Close()

			client := newTestClient(t, srv)
			prepareForWait(t, client)

			status, err := client.WaitForPipeline(t.Context(), 30*time.Second)
			if err != nil {
				t.Fatalf("WaitForPipeline: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
		})
	}
}

// TestWaitForPipeline_TransitionsThroughPending drives the poll loop over several
// rounds, which is where the status tracker's transitions run.
func TestWaitForPipeline_TransitionsThroughPending(t *testing.T) {
	srv := statusStub(t, "success", 2)
	defer srv.Close()

	client := newTestClient(t, srv)
	prepareForWait(t, client)

	status, err := client.WaitForPipeline(t.Context(), 60*time.Second)
	if err != nil {
		t.Fatalf("WaitForPipeline: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
	}
}

// TestWaitForPipeline_NoStatusesIsTreatedAsNoCI covers the grace period: a repository
// with no CI reports no statuses at all, which must eventually be read as "nothing to
// wait for" rather than as a pipeline that is still starting.
func TestWaitForPipeline_NoStatusesIsTreatedAsNoCI(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "", "total_count": 0, "statuses": []any{}})
		case strings.HasSuffix(r.URL.Path, "/pulls") && r.Method == http.MethodPost:
			writeJSON(t, w, map[string]any{
				"number": 42,
				"head":   map[string]any{"ref": "feature", "sha": "deadbeef"},
				"base":   map[string]any{"ref": "main"},
			})
		default:
			writeJSON(t, w, map[string]any{"name": "repo", "full_name": "owner/repo"})
		}
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	prepareForWait(t, client)

	status, err := client.WaitForPipeline(t.Context(), 60*time.Second)
	if err != nil {
		t.Fatalf("WaitForPipeline: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success for a repository with no CI", status)
	}
}

// TestWaitForPipeline_TimesOut verifies the budget is enforced when statuses never
// resolve.
func TestWaitForPipeline_TimesOut(t *testing.T) {
	srv := statusStub(t, "success", 1_000_000)
	defer srv.Close()

	client := newTestClient(t, srv)
	prepareForWait(t, client)

	start := time.Now()
	_, err := client.WaitForPipeline(t.Context(), 2*time.Second)

	if !errors.Is(err, forgejo.ErrWorkflowTimeout) {
		t.Errorf("got %v, want ErrWorkflowTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("took %v, far beyond the 2s budget", elapsed)
	}
}

// TestMergePullRequest_RejectedIsNotReportedAsSuccess pins the sentinel for a merge
// the server declined.
//
// The gitea SDK signals a rejected merge through its boolean result rather than an
// error: a non-2xx response comes back as (false, resp, nil). Ignoring that reported
// success for a merge that never happened, after which auto-mr proceeds to delete the
// branch that still holds the work.
func TestMergePullRequest_RejectedIsNotReportedAsSuccess(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/merge") {
			// Forgejo answers an unmergeable pull request this way.
			w.WriteHeader(http.StatusMethodNotAllowed)
			writeJSON(t, w, map[string]any{"message": "Please try again later"})
			return
		}
		writeJSON(t, w, map[string]any{})
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	setRepo(t, client)

	err := client.MergePullRequest(t.Context(), 42, true, "Add a feature")
	if !errors.Is(err, forgejo.ErrMergeRejected) {
		t.Errorf("got %v, want it to wrap ErrMergeRejected", err)
	}
}

// TestMergePullRequest_Success confirms the happy path still passes, so the check
// above cannot be satisfied by rejecting every merge.
func TestMergePullRequest_Success(t *testing.T) {
	srv := forgejoStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/merge") {
			w.WriteHeader(http.StatusOK)
			return
		}
		writeJSON(t, w, map[string]any{})
	})
	defer srv.Close()

	client := newTestClient(t, srv)
	setRepo(t, client)

	if err := client.MergePullRequest(t.Context(), 42, true, "Add a feature"); err != nil {
		t.Errorf("MergePullRequest: %v", err)
	}
}

// TestMain shortens the poll cadence for this package's tests.
//
// The production schedule waits five seconds between polls, which is right for real
// CI but makes multi-round wait tests dominate the suite's runtime. The intervals
// under test are the schedule's own concern and are covered directly in
// internal/polling; here only the loop behaviour matters.
func TestMain(m *testing.M) {
	polling.DefaultSchedule = polling.Schedule{
		Base:      10 * time.Millisecond,
		Grown:     10 * time.Millisecond,
		Cap:       10 * time.Millisecond,
		GrowAfter: time.Hour,
		CapAfter:  time.Hour,
	}
	os.Exit(m.Run())
}
