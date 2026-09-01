package gitlab_test

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

	"github.com/sgaunet/auto-mr/pkg/gitlab"
)

// These tests drive the real *gitlab.Client against a stub API rather than a fake
// that returns whatever it was handed. That distinction matters: the previous suite
// exercised only testing/mocks, so the client's own request building, error
// classification and pipeline aggregation were never executed.

// newTestClient returns a client whose SDK talks to srv.
func newTestClient(t *testing.T, srv *httptest.Server) *gitlab.Client {
	t.Helper()

	t.Setenv("GITLAB_TOKEN", "glpat-httptestclient12345")
	client, err := gitlab.NewClient(gitlab.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// writeJSON serialises v as the response body, failing the test if it cannot.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

// userRoutes serves the assignee and reviewer lookups CreateMergeRequest performs
// before it can build its request.
func userRoutes(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	writeJSON(t, w, []map[string]any{{"id": 1, "username": "alice"}})
}

func TestCreateMergeRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			userRoutes(t, w)
		case strings.HasSuffix(r.URL.Path, "/merge_requests") && r.Method == http.MethodPost:
			writeJSON(t, w, map[string]any{
				"iid":           7,
				"sha":           "deadbeef",
				"web_url":       "https://gitlab.example/o/r/-/merge_requests/7",
				"source_branch": "feature",
			})
		default:
			writeJSON(t, w, map[string]any{"id": 99, "path_with_namespace": "o/r"})
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	mr, err := client.CreateMergeRequest(t.Context(),
		"feature", "main", "Add a feature", "body", "alice", "bob",
		[]string{"bug"}, true)
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}

	if mr.IID != 7 {
		t.Errorf("IID = %d, want 7", mr.IID)
	}
	if mr.SourceBranch != "feature" {
		t.Errorf("SourceBranch = %q, want feature", mr.SourceBranch)
	}
}

// TestCreateMergeRequest_AlreadyExists covers the classification main.go depends on
// to recover an existing merge request. GitLab reports this as a 409 whose message
// has to be inspected, so the mapping is string-based and worth pinning.
func TestCreateMergeRequest_AlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			userRoutes(t, w)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/merge_requests") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			writeJSON(t, w, map[string]any{
				"message": []string{"Another open merge request already exists for this source branch"},
			})
			return
		}
		writeJSON(t, w, map[string]any{"id": 99})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.CreateMergeRequest(t.Context(),
		"feature", "main", "t", "b", "alice", "bob", nil, true)

	if !errors.Is(err, gitlab.ErrMRAlreadyExists) {
		t.Errorf("got %v, want it to wrap ErrMRAlreadyExists", err)
	}
}

func TestCreateMergeRequest_AssigneeNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			writeJSON(t, w, []map[string]any{}) // No such user.
			return
		}
		writeJSON(t, w, map[string]any{"id": 99})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.CreateMergeRequest(t.Context(),
		"feature", "main", "t", "b", "ghost", "bob", nil, true)

	if !errors.Is(err, gitlab.ErrAssigneeNotFound) {
		t.Errorf("got %v, want it to wrap ErrAssigneeNotFound", err)
	}
}

// waitForPipelineHarness serves the endpoints a pipeline wait needs, reporting the
// given job status once the requested number of polls have been observed.
func waitForPipelineHarness(t *testing.T, jobStatus string, pollsBeforeTerminal int32) *httptest.Server {
	t.Helper()

	var polls atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			userRoutes(t, w)

		case strings.HasSuffix(r.URL.Path, "/merge_requests") && r.Method == http.MethodPost:
			writeJSON(t, w, map[string]any{"iid": 7, "sha": "deadbeef", "source_branch": "feature"})

		// hasPipelineRuns: report one so the wait does not short-circuit.
		case strings.HasSuffix(r.URL.Path, "/pipelines") && strings.Contains(r.URL.Path, "/projects/") &&
			!strings.Contains(r.URL.Path, "/merge_requests/"):
			writeJSON(t, w, []map[string]any{{"id": 11, "status": "running", "sha": "deadbeef"}})

		case strings.HasSuffix(r.URL.Path, "/pipelines"):
			writeJSON(t, w, []map[string]any{{"id": 11, "status": "running", "sha": "deadbeef"}})

		case strings.HasSuffix(r.URL.Path, "/jobs"):
			// Report a running job until enough polls have happened, then terminal.
			status := "running"
			if polls.Add(1) > pollsBeforeTerminal {
				status = jobStatus
			}
			writeJSON(t, w, []map[string]any{{
				"id": 21, "name": "build", "stage": "test", "status": status,
			}})

		default:
			writeJSON(t, w, map[string]any{"id": 99})
		}
	}))
}

// createMRForWait puts the client into the state a pipeline wait requires: the
// merge request IID and SHA are set by creating it.
func createMRForWait(t *testing.T, client *gitlab.Client) {
	t.Helper()

	if _, err := client.CreateMergeRequest(t.Context(),
		"feature", "main", "t", "b", "alice", "bob", nil, true); err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
}

func TestWaitForPipeline_Outcomes(t *testing.T) {
	tests := []struct {
		name       string
		jobStatus  string
		wantStatus string
	}{
		{name: "success", jobStatus: "success", wantStatus: "success"},
		{name: "failed", jobStatus: "failed", wantStatus: "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := waitForPipelineHarness(t, tt.jobStatus, 0)
			defer srv.Close()

			client := newTestClient(t, srv)
			createMRForWait(t, client)

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

// TestWaitForPipeline_TransitionsThroughRunning exercises the poll loop across more
// than one round, which is where the tracker's state transitions actually run.
func TestWaitForPipeline_TransitionsThroughRunning(t *testing.T) {
	srv := waitForPipelineHarness(t, "success", 1)
	defer srv.Close()

	client := newTestClient(t, srv)
	createMRForWait(t, client)

	status, err := client.WaitForPipeline(t.Context(), 60*time.Second)
	if err != nil {
		t.Fatalf("WaitForPipeline: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
	}
}

// TestWaitForPipeline_TimesOut verifies the budget is enforced against a pipeline
// that never finishes, and that the timeout sentinel is reported.
func TestWaitForPipeline_TimesOut(t *testing.T) {
	// A high poll count means the job never reaches a terminal state.
	srv := waitForPipelineHarness(t, "success", 1_000_000)
	defer srv.Close()

	client := newTestClient(t, srv)
	createMRForWait(t, client)

	start := time.Now()
	_, err := client.WaitForPipeline(t.Context(), 2*time.Second)

	if !errors.Is(err, gitlab.ErrPipelineTimeout) {
		t.Errorf("got %v, want ErrPipelineTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("took %v, far beyond the 2s budget", elapsed)
	}
}

func TestApproveAndMergeMergeRequest(t *testing.T) {
	var approved, merged atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/approve"):
			approved.Store(true)
			writeJSON(t, w, map[string]any{"id": 1})
		case strings.HasSuffix(r.URL.Path, "/merge"):
			merged.Store(true)
			writeJSON(t, w, map[string]any{"iid": 7, "state": "merged"})
		default:
			writeJSON(t, w, map[string]any{"id": 99})
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)

	if err := client.ApproveMergeRequest(t.Context(), 7); err != nil {
		t.Fatalf("ApproveMergeRequest: %v", err)
	}
	if err := client.MergeMergeRequest(t.Context(), 7, true, "Add a feature"); err != nil {
		t.Fatalf("MergeMergeRequest: %v", err)
	}

	if !approved.Load() {
		t.Error("approve endpoint was never called")
	}
	if !merged.Load() {
		t.Error("merge endpoint was never called")
	}
}

// TestMergeMergeRequest_SurfacesFailure guards the most consequential error path: a
// merge that the server rejected must not be reported as done.
func TestMergeMergeRequest_SurfacesFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/merge") {
			w.WriteHeader(http.StatusMethodNotAllowed)
			writeJSON(t, w, map[string]any{"message": "Branch cannot be merged"})
			return
		}
		writeJSON(t, w, map[string]any{"id": 99})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	if err := client.MergeMergeRequest(t.Context(), 7, true, "t"); err == nil {
		t.Error("expected an error for a rejected merge")
	}
}

func TestListLabels_ReturnsNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/labels") {
			writeJSON(t, w, []map[string]any{{"name": "bug"}, {"name": "enhancement"}})
			return
		}
		writeJSON(t, w, map[string]any{"id": 99})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	labels, err := client.ListLabels(t.Context())
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 2 || labels[0].Name != "bug" {
		t.Errorf("labels = %+v", labels)
	}
}

// TestGetMergeRequestByBranch_NotFound covers the sentinel the adapter maps onto the
// shared contract.
func TestGetMergeRequestByBranch_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/merge_requests") {
			writeJSON(t, w, []map[string]any{}) // No open merge requests.
			return
		}
		writeJSON(t, w, map[string]any{"id": 99})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.GetMergeRequestByBranch(t.Context(), "feature", "main")

	if !errors.Is(err, gitlab.ErrMRNotFound) {
		t.Errorf("got %v, want ErrMRNotFound", err)
	}
}

// TestFetchPipelineJobs_Paginates verifies the job fetch follows GitLab's pagination
// headers instead of silently reporting only the first page, which would make a
// pipeline look complete while later jobs were still running.
func TestFetchPipelineJobs_Paginates(t *testing.T) {
	var jobPages atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			userRoutes(t, w)

		case strings.HasSuffix(r.URL.Path, "/merge_requests") && r.Method == http.MethodPost:
			writeJSON(t, w, map[string]any{"iid": 7, "sha": "deadbeef", "source_branch": "feature"})

		case strings.HasSuffix(r.URL.Path, "/jobs"):
			page := jobPages.Add(1)
			if page == 1 {
				// Advertise a second page; the first job is already finished.
				w.Header().Set("X-Next-Page", "2")
				w.Header().Set("X-Page", "1")
				writeJSON(t, w, []map[string]any{{
					"id": 21, "name": "build", "stage": "build", "status": "success",
				}})
				return
			}
			w.Header().Set("X-Next-Page", "")
			writeJSON(t, w, []map[string]any{{
				"id": 22, "name": "test", "stage": "test", "status": "success",
			}})

		case strings.HasSuffix(r.URL.Path, "/pipelines"):
			writeJSON(t, w, []map[string]any{{"id": 11, "status": "running", "sha": "deadbeef"}})

		default:
			writeJSON(t, w, map[string]any{"id": 99})
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	createMRForWait(t, client)

	status, err := client.WaitForPipeline(t.Context(), 30*time.Second)
	if err != nil {
		t.Fatalf("WaitForPipeline: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
	}
	if got := jobPages.Load(); got < 2 {
		t.Errorf("job endpoint saw %d pages; pagination was not followed", got)
	}
}

// TestSetProjectFromURL_DerivesProjectPath exercises the real URL parsing rather
// than a mock's stored answer.
func TestSetProjectFromURL_DerivesProjectPath(t *testing.T) {
	var requested atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested.Store(r.URL.Path)
		writeJSON(t, w, map[string]any{"id": 99, "path_with_namespace": "owner/repo"})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	if err := client.SetProjectFromURL(t.Context(), "git@gitlab.com:owner/repo.git"); err != nil {
		t.Fatalf("SetProjectFromURL: %v", err)
	}

	path, _ := requested.Load().(string)
	if !strings.Contains(path, "owner") {
		t.Errorf("requested path %q does not name the project derived from the remote", path)
	}
}

// TestWaitForPipeline_JobWithoutCreatedAt guards a crash found by exercising the
// real client: created_at is optional in the API response, and dereferencing it
// unconditionally panicked with a nil pointer dereference, taking down the whole run
// on a job that had not started yet. A mock returning pre-built Job values could
// never surface this, because the conversion from the SDK type is where it happened.
func TestWaitForPipeline_JobWithoutCreatedAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			userRoutes(t, w)
		case strings.HasSuffix(r.URL.Path, "/merge_requests") && r.Method == http.MethodPost:
			writeJSON(t, w, map[string]any{"iid": 7, "sha": "deadbeef", "source_branch": "feature"})
		case strings.HasSuffix(r.URL.Path, "/jobs"):
			// Deliberately omits created_at, started_at and finished_at.
			writeJSON(t, w, []map[string]any{{
				"id": 21, "name": "build", "stage": "test", "status": "success",
			}})
		case strings.HasSuffix(r.URL.Path, "/pipelines"):
			writeJSON(t, w, []map[string]any{{"id": 11, "status": "running", "sha": "deadbeef"}})
		default:
			writeJSON(t, w, map[string]any{"id": 99})
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	createMRForWait(t, client)

	status, err := client.WaitForPipeline(t.Context(), 30*time.Second)
	if err != nil {
		t.Fatalf("WaitForPipeline: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
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
