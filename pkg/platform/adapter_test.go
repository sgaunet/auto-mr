package platform_test

import (
	"errors"
	"testing"

	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/sgaunet/auto-mr/pkg/platform"
	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
)

// The adapters are the layer that translates platform-specific errors into the
// shared Provider contract. Until the adapters held the APIClient interface rather
// than a concrete client there was no way to substitute a client, so none of this
// translation could be tested -- the existing tests exercised the mock directly and
// never reached adapter code at all.

func newGitLabAdapter(mockAPI *mocks.GitLabAPIClient) *platform.GitLabAdapter {
	return platform.NewGitLabAdapter(mockAPI, config.GitLabConfig{
		Assignee: "alice",
		Reviewer: "bob",
	}, logger.NoLogger())
}

// TestGitLabAdapter_CreateMapsAlreadyExists covers the branch main.go depends on: on
// ErrAlreadyExists it fetches the existing merge request instead of failing, so if
// this mapping breaks, a re-run against an existing branch aborts rather than
// recovering.
func TestGitLabAdapter_CreateMapsAlreadyExists(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.CreateMergeRequestError = gitlab.ErrMRAlreadyExists

	_, err := newGitLabAdapter(mockAPI).Create(t.Context(), platform.CreateParams{
		SourceBranch: "feature",
		TargetBranch: "main",
		Title:        "Add a feature",
	})

	if !errors.Is(err, platform.ErrAlreadyExists) {
		t.Errorf("got %v, want it to wrap platform.ErrAlreadyExists", err)
	}
	// The underlying cause must survive the translation for diagnostics.
	if !errors.Is(err, gitlab.ErrMRAlreadyExists) {
		t.Errorf("got %v, want it to still wrap gitlab.ErrMRAlreadyExists", err)
	}
}

// TestGitLabAdapter_CreateOtherErrorIsNotAlreadyExists guards against a mapping so
// broad that unrelated failures are reported as "already exists", which would send
// main.go looking for a merge request that does not exist.
func TestGitLabAdapter_CreateOtherErrorIsNotAlreadyExists(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.CreateMergeRequestError = gitlab.ErrTokenRequired

	_, err := newGitLabAdapter(mockAPI).Create(t.Context(), platform.CreateParams{
		SourceBranch: "feature",
		TargetBranch: "main",
	})

	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, platform.ErrAlreadyExists) {
		t.Errorf("an unrelated error was mapped to ErrAlreadyExists: %v", err)
	}
}

// TestGitLabAdapter_CreatePassesConfiguredAssigneeAndReviewer verifies the adapter
// supplies these from config rather than from the caller's parameters, which is the
// whole reason the adapter holds a config.
func TestGitLabAdapter_CreatePassesConfiguredAssigneeAndReviewer(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()

	if _, err := newGitLabAdapter(mockAPI).Create(t.Context(), platform.CreateParams{
		SourceBranch: "feature",
		TargetBranch: "main",
		Title:        "Add a feature",
		Labels:       []string{"bug"},
		Squash:       true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	call := mockAPI.GetLastCall("CreateMergeRequest")
	if call == nil {
		t.Fatal("adapter did not reach the client; the interface seam is not wired")
	}
	if got := call.Args["assignee"]; got != "alice" {
		t.Errorf("assignee = %v, want alice", got)
	}
	if got := call.Args["reviewer"]; got != "bob" {
		t.Errorf("reviewer = %v, want bob", got)
	}
}

// TestGitLabAdapter_CreateReturnsMappedFields verifies the translation to the
// platform-agnostic type, which is what main.go prints and later merges.
func TestGitLabAdapter_CreateReturnsMappedFields(t *testing.T) {
	want := fixtures.ValidMergeRequest()
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.CreateMergeRequestResponse = want

	got, err := newGitLabAdapter(mockAPI).Create(t.Context(), platform.CreateParams{
		SourceBranch: "feature",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got.ID != want.IID {
		t.Errorf("ID = %d, want %d", got.ID, want.IID)
	}
	if got.WebURL != want.WebURL {
		t.Errorf("WebURL = %q, want %q", got.WebURL, want.WebURL)
	}
	if got.SourceBranch != want.SourceBranch {
		t.Errorf("SourceBranch = %q, want %q", got.SourceBranch, want.SourceBranch)
	}
}

// TestGitLabAdapter_PlatformNameAndTimeout covers the two metadata methods; the
// timeout string feeds main.go's flag-over-config resolution.
func TestGitLabAdapter_PlatformNameAndTimeout(t *testing.T) {
	adapter := platform.NewGitLabAdapter(mocks.NewGitLabAPIClient(), config.GitLabConfig{
		PipelineTimeout: "45m",
	}, logger.NoLogger())

	if got := adapter.PlatformName(); got != "GitLab" {
		t.Errorf("PlatformName = %q, want GitLab", got)
	}
	if got := adapter.PipelineTimeout(); got != "45m" {
		t.Errorf("PipelineTimeout = %q, want 45m", got)
	}
}
