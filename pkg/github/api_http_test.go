package github_test

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

	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
)

// These tests drive the real *github.Client against a stub API, so request building
// and error classification actually run. The suite they replace configured a mock and
// asserted the mock returned what it had been given.

func newTestClient(t *testing.T, srv *httptest.Server) *ghpkg.Client {
	t.Helper()

	t.Setenv("GITHUB_TOKEN", "ghp_httptestclient1234567890123456")
	client, err := ghpkg.NewClient(t.Context())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := ghpkg.SetBaseURLForTest(client, srv.URL+"/"); err != nil {
		t.Fatalf("SetBaseURLForTest: %v", err)
	}
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func TestSetRepositoryFromURL_DerivesOwnerAndRepo(t *testing.T) {
	var requested atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested.Store(r.URL.Path)
		writeJSON(t, w, map[string]any{"name": "repo", "full_name": "owner/repo"})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	if err := client.SetRepositoryFromURL(t.Context(), "git@github.com:owner/repo.git"); err != nil {
		t.Fatalf("SetRepositoryFromURL: %v", err)
	}

	path, _ := requested.Load().(string)
	if !strings.Contains(path, "owner/repo") {
		t.Errorf("requested %q, which does not name the repository from the remote", path)
	}
}

func TestListLabels_ReturnsNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/labels") {
			writeJSON(t, w, []map[string]any{{"name": "bug"}, {"name": "enhancement"}})
			return
		}
		writeJSON(t, w, map[string]any{})
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

func TestCreatePullRequest_Success(t *testing.T) {
	var addedAssignees, requestedReviewers atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls") && r.Method == http.MethodPost:
			writeJSON(t, w, map[string]any{
				"number":   42,
				"html_url": "https://github.com/o/r/pull/42",
				"head":     map[string]any{"ref": "feature", "sha": "deadbeef"},
				"base":     map[string]any{"ref": "main"},
				"user":     map[string]any{"login": "author"},
			})
		case strings.Contains(r.URL.Path, "/assignees"):
			addedAssignees.Store(true)
			writeJSON(t, w, map[string]any{"number": 42})
		case strings.Contains(r.URL.Path, "/requested_reviewers"):
			requestedReviewers.Store(true)
			writeJSON(t, w, map[string]any{"number": 42})
		case strings.Contains(r.URL.Path, "/labels"):
			writeJSON(t, w, []map[string]any{{"name": "bug"}})
		default:
			writeJSON(t, w, map[string]any{})
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	pr, err := client.CreatePullRequest(t.Context(),
		"feature", "main", "Add a feature", "body",
		[]string{"alice"}, []string{"bob"}, []string{"bug"})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	if pr.GetNumber() != 42 {
		t.Errorf("Number = %d, want 42", pr.GetNumber())
	}
	if !addedAssignees.Load() {
		t.Error("assignees were never added")
	}
	if !requestedReviewers.Load() {
		t.Error("reviewers were never requested")
	}
}

// TestCreatePullRequest_AlreadyExists covers the classification main.go relies on to
// recover an existing pull request instead of failing. GitHub reports this as a 422
// whose message must be inspected.
func TestCreatePullRequest_AlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusUnprocessableEntity)
			writeJSON(t, w, map[string]any{
				"message": "Validation Failed",
				"errors": []map[string]any{
					{"message": "A pull request already exists for owner:feature."},
				},
			})
			return
		}
		writeJSON(t, w, map[string]any{})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.CreatePullRequest(t.Context(), "feature", "main", "t", "b", nil, nil, nil)

	if !errors.Is(err, ghpkg.ErrPRAlreadyExists) {
		t.Errorf("got %v, want it to wrap ErrPRAlreadyExists", err)
	}
}

// TestCreatePullRequest_SkipsAuthorAsReviewer covers a rule GitHub enforces itself:
// requesting the author as a reviewer is rejected, so the client filters them out.
func TestCreatePullRequest_SkipsAuthorAsReviewer(t *testing.T) {
	var reviewerCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls") && r.Method == http.MethodPost:
			writeJSON(t, w, map[string]any{
				"number": 42,
				"head":   map[string]any{"ref": "feature"},
				"user":   map[string]any{"login": "alice"},
			})
		case strings.Contains(r.URL.Path, "/requested_reviewers"):
			reviewerCalls.Add(1)
			writeJSON(t, w, map[string]any{"number": 42})
		default:
			writeJSON(t, w, map[string]any{})
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	// alice authored the pull request and is also the configured reviewer.
	if _, err := client.CreatePullRequest(t.Context(),
		"feature", "main", "t", "b", nil, []string{"alice"}, nil); err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	if reviewerCalls.Load() != 0 {
		t.Error("the pull request author was requested as a reviewer, which GitHub rejects")
	}
}

func TestGetPullRequestByBranch_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls") {
			writeJSON(t, w, []map[string]any{}) // No open pull requests.
			return
		}
		writeJSON(t, w, map[string]any{})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.GetPullRequestByBranch(t.Context(), "feature", "main")

	if !errors.Is(err, ghpkg.ErrPRNotFound) {
		t.Errorf("got %v, want ErrPRNotFound", err)
	}
}

func TestGetPullRequestByBranch_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls") {
			writeJSON(t, w, []map[string]any{{
				"number": 42,
				"head":   map[string]any{"ref": "feature"},
				"base":   map[string]any{"ref": "main"},
			}})
			return
		}
		writeJSON(t, w, map[string]any{})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	pr, err := client.GetPullRequestByBranch(t.Context(), "feature", "main")
	if err != nil {
		t.Fatalf("GetPullRequestByBranch: %v", err)
	}
	if pr.GetNumber() != 42 {
		t.Errorf("Number = %d, want 42", pr.GetNumber())
	}
}

func TestMergePullRequest_Success(t *testing.T) {
	var merged atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/merge") {
			merged.Store(true)
			writeJSON(t, w, map[string]any{"merged": true, "sha": "deadbeef"})
			return
		}
		writeJSON(t, w, map[string]any{})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	if err := client.MergePullRequest(t.Context(), 42, "squash", "Add a feature"); err != nil {
		t.Fatalf("MergePullRequest: %v", err)
	}
	if !merged.Load() {
		t.Error("merge endpoint was never called")
	}
}

// TestMergePullRequest_SurfacesFailure guards the most consequential error path: a
// merge the server refused must not be reported as done, or the caller will clean up
// a branch whose work was never merged.
func TestMergePullRequest_SurfacesFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/merge") {
			w.WriteHeader(http.StatusMethodNotAllowed)
			writeJSON(t, w, map[string]any{"message": "Pull Request is not mergeable"})
			return
		}
		writeJSON(t, w, map[string]any{})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	if err := client.MergePullRequest(t.Context(), 42, "squash", "t"); err == nil {
		t.Error("expected an error for a rejected merge")
	}
}

func TestDeleteBranch(t *testing.T) {
	var deleted atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted.Store(r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(t, w, map[string]any{})
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	if err := client.DeleteBranch(t.Context(), "feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	path, _ := deleted.Load().(string)
	if !strings.Contains(path, "feature") {
		t.Errorf("deleted ref path %q does not name the branch", path)
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
