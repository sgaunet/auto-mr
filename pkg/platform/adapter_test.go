package platform_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/forgejo"
	ghclient "github.com/sgaunet/auto-mr/pkg/github"
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

// --- GitHub adapter ---

func newGitHubAdapter(mockAPI *mocks.GitHubAPIClient) *platform.GitHubAdapter {
	return platform.NewGitHubAdapter(mockAPI, config.GitHubConfig{
		Assignee: "alice",
		Reviewer: "bob",
	}, logger.NoLogger())
}

func TestGitHubAdapter_CreateMapsAlreadyExists(t *testing.T) {
	mockAPI := mocks.NewGitHubAPIClient()
	mockAPI.CreatePullRequestError = ghclient.ErrPRAlreadyExists

	_, err := newGitHubAdapter(mockAPI).Create(t.Context(), platform.CreateParams{
		SourceBranch: "feature",
		TargetBranch: "main",
	})

	if !errors.Is(err, platform.ErrAlreadyExists) {
		t.Errorf("got %v, want it to wrap platform.ErrAlreadyExists", err)
	}
}

// TestGitHubAdapter_ApproveIsNoOp documents that GitHub has no approval step, so the
// shared Provider contract is satisfied without an API call.
func TestGitHubAdapter_ApproveIsNoOp(t *testing.T) {
	mockAPI := mocks.NewGitHubAPIClient()

	if err := newGitHubAdapter(mockAPI).Approve(t.Context(), 1); err != nil {
		t.Errorf("Approve returned %v, want nil", err)
	}
	if calls := mockAPI.GetCalls(); len(calls) != 0 {
		t.Errorf("Approve made %d API calls, want none", len(calls))
	}
}

// TestGitHubAdapter_MergeDeletesSourceBranch covers the extra step GitHub performs
// that the other platforms handle server-side.
func TestGitHubAdapter_MergeDeletesSourceBranch(t *testing.T) {
	mockAPI := mocks.NewGitHubAPIClient()

	if err := newGitHubAdapter(mockAPI).Merge(t.Context(), platform.MergeParams{
		MRID:         7,
		Squash:       true,
		CommitTitle:  "Add a feature",
		SourceBranch: "feature",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if mockAPI.GetCallCount("MergePullRequest") != 1 {
		t.Error("Merge did not call MergePullRequest")
	}
	deleted := mockAPI.GetLastCall("DeleteBranch")
	if deleted == nil {
		t.Fatal("Merge did not delete the source branch")
	}
	if got := deleted.Args["branch"]; got != "feature" {
		t.Errorf("deleted branch %v, want feature", got)
	}
}

// TestGitHubAdapter_MergeSurvivesBranchDeletionFailure verifies that a failed branch
// deletion does not fail the merge. The merge has already happened at that point, so
// reporting failure would misrepresent the outcome and prompt a pointless retry.
func TestGitHubAdapter_MergeSurvivesBranchDeletionFailure(t *testing.T) {
	mockAPI := mocks.NewGitHubAPIClient()
	mockAPI.DeleteBranchError = errors.New("branch is protected")

	if err := newGitHubAdapter(mockAPI).Merge(t.Context(), platform.MergeParams{
		MRID:         7,
		SourceBranch: "feature",
	}); err != nil {
		t.Errorf("Merge failed because branch deletion failed: %v", err)
	}
}

// --- Forgejo adapter ---

func newForgejoAdapter(mockAPI *mocks.ForgejoAPIClient) *platform.ForgejoAdapter {
	return platform.NewForgejoAdapter(mockAPI, config.ForgejoConfig{
		URL:      "https://forgejo.example.com",
		Assignee: "alice",
		Reviewer: "bob",
	}, logger.NoLogger())
}

func TestForgejoAdapter_CreateMapsAlreadyExists(t *testing.T) {
	mockAPI := mocks.NewForgejoAPIClient()
	mockAPI.CreatePullRequestError = forgejo.ErrPRAlreadyExists

	_, err := newForgejoAdapter(mockAPI).Create(t.Context(), platform.CreateParams{
		SourceBranch: "feature",
		TargetBranch: "main",
	})

	if !errors.Is(err, platform.ErrAlreadyExists) {
		t.Errorf("got %v, want it to wrap platform.ErrAlreadyExists", err)
	}
}

func TestForgejoAdapter_CreateReturnsMappedFields(t *testing.T) {
	want := fixtures.ValidForgejoPullRequest()
	mockAPI := mocks.NewForgejoAPIClient()
	mockAPI.CreatePullRequestResponse = want

	got, err := newForgejoAdapter(mockAPI).Create(t.Context(), platform.CreateParams{
		SourceBranch: "feature",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got.ID != want.Index {
		t.Errorf("ID = %d, want %d", got.ID, want.Index)
	}
	if got.WebURL != want.HTMLURL {
		t.Errorf("WebURL = %q, want %q", got.WebURL, want.HTMLURL)
	}
	if got.SourceBranch != want.Head.Ref {
		t.Errorf("SourceBranch = %q, want %q", got.SourceBranch, want.Head.Ref)
	}
}

func TestForgejoAdapter_ListLabelsMapsNames(t *testing.T) {
	mockAPI := mocks.NewForgejoAPIClient()
	mockAPI.ListLabelsResponse = fixtures.ValidForgejoLabels()

	labels, err := newForgejoAdapter(mockAPI).ListLabels(t.Context())
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != len(mockAPI.ListLabelsResponse) {
		t.Fatalf("got %d labels, want %d", len(labels), len(mockAPI.ListLabelsResponse))
	}
	if labels[0].Name != mockAPI.ListLabelsResponse[0].Name {
		t.Errorf("label = %q, want %q", labels[0].Name, mockAPI.ListLabelsResponse[0].Name)
	}
}

func TestForgejoAdapter_WaitForPipelineForwardsTimeout(t *testing.T) {
	mockAPI := mocks.NewForgejoAPIClient()
	mockAPI.WaitForPipelineStatus = "success"

	status, err := newForgejoAdapter(mockAPI).WaitForPipeline(t.Context(), 90*time.Second)
	if err != nil {
		t.Fatalf("WaitForPipeline: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
	}

	call := mockAPI.GetLastCall("WaitForPipeline")
	if call == nil {
		t.Fatal("adapter did not reach the client")
	}
	if got := call.Args["timeout"]; got != 90*time.Second {
		t.Errorf("timeout = %v, want 90s", got)
	}
}

// --- Pass-through and error-wrapping coverage for the remaining methods ---

// Each adapter method translates between the shared contract and one platform's
// client. The behaviour worth pinning is that arguments arrive intact, results are
// mapped onto the shared types, and a client error is surfaced rather than swallowed.

func TestGitLabAdapter_InitializeForwardsRemoteURL(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()

	if err := newGitLabAdapter(mockAPI).Initialize(t.Context(), "https://gitlab.com/o/r.git"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	call := mockAPI.GetLastCall("SetProjectFromURL")
	if call == nil {
		t.Fatal("adapter did not reach the client")
	}
	if got := call.Args["url"]; got != "https://gitlab.com/o/r.git" {
		t.Errorf("url = %v", got)
	}
}

func TestGitLabAdapter_InitializeSurfacesError(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.SetProjectFromURLError = gitlab.ErrInvalidURLFormat

	err := newGitLabAdapter(mockAPI).Initialize(t.Context(), "not-a-url")
	if !errors.Is(err, gitlab.ErrInvalidURLFormat) {
		t.Errorf("got %v, want it to wrap gitlab.ErrInvalidURLFormat", err)
	}
}

func TestGitLabAdapter_ListLabelsMapsNames(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.ListLabelsResponse = []*gitlab.Label{{Name: "bug"}, {Name: "enhancement"}}

	labels, err := newGitLabAdapter(mockAPI).ListLabels(t.Context())
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 2 || labels[0].Name != "bug" || labels[1].Name != "enhancement" {
		t.Errorf("labels = %+v, want bug and enhancement", labels)
	}
}

func TestGitLabAdapter_ListLabelsSurfacesError(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.ListLabelsError = gitlab.ErrTokenRequired

	if _, err := newGitLabAdapter(mockAPI).ListLabels(t.Context()); err == nil {
		t.Error("expected an error")
	}
}

func TestGitLabAdapter_GetByBranchMapsFields(t *testing.T) {
	want := fixtures.ValidMergeRequest()
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.GetMergeRequestByBranchResponse = want

	got, err := newGitLabAdapter(mockAPI).GetByBranch(t.Context(), "feature", "main")
	if err != nil {
		t.Fatalf("GetByBranch: %v", err)
	}
	if got.ID != want.IID || got.WebURL != want.WebURL {
		t.Errorf("got %+v, want ID %d and URL %q", got, want.IID, want.WebURL)
	}
}

func TestGitLabAdapter_GetByBranchSurfacesNotFound(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.GetMergeRequestByBranchError = gitlab.ErrMRNotFound

	if _, err := newGitLabAdapter(mockAPI).GetByBranch(t.Context(), "feature", "main"); err == nil {
		t.Error("expected an error")
	}
}

func TestGitLabAdapter_ApproveAndMergeForwardArguments(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	adapter := newGitLabAdapter(mockAPI)

	if err := adapter.Approve(t.Context(), 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := adapter.Merge(t.Context(), platform.MergeParams{
		MRID:        42,
		Squash:      true,
		CommitTitle: "Add a feature",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	call := mockAPI.GetLastCall("MergeMergeRequest")
	if call == nil {
		t.Fatal("adapter did not reach the client")
	}
	if got := call.Args["squash"]; got != true {
		t.Errorf("squash = %v, want true", got)
	}
	if got := call.Args["commitTitle"]; got != "Add a feature" {
		t.Errorf("commitTitle = %v", got)
	}
}

func TestGitLabAdapter_MergeSurfacesError(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.MergeMergeRequestError = errors.New("merge conflict")

	if err := newGitLabAdapter(mockAPI).Merge(t.Context(), platform.MergeParams{MRID: 1}); err == nil {
		t.Error("expected an error; a failed merge must not report success")
	}
}

func TestGitLabAdapter_WaitForPipelineForwardsTimeout(t *testing.T) {
	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.WaitForPipelineStatus = "success"

	status, err := newGitLabAdapter(mockAPI).WaitForPipeline(t.Context(), 45*time.Second)
	if err != nil {
		t.Fatalf("WaitForPipeline: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
	}
	if call := mockAPI.GetLastCall("WaitForPipeline"); call == nil {
		t.Fatal("adapter did not reach the client")
	} else if got := call.Args["timeout"]; got != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", got)
	}
}

func TestGitHubAdapter_InitializeAndListLabels(t *testing.T) {
	mockAPI := mocks.NewGitHubAPIClient()
	mockAPI.ListLabelsResponse = []*ghclient.Label{{Name: "bug"}}
	adapter := newGitHubAdapter(mockAPI)

	if err := adapter.Initialize(t.Context(), "https://github.com/o/r.git"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	labels, err := adapter.ListLabels(t.Context())
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 1 || labels[0].Name != "bug" {
		t.Errorf("labels = %+v", labels)
	}
}

func TestGitHubAdapter_GetByBranchMapsFields(t *testing.T) {
	want := fixtures.ValidPullRequest()
	mockAPI := mocks.NewGitHubAPIClient()
	mockAPI.GetPullRequestByBranchResponse = want

	got, err := newGitHubAdapter(mockAPI).GetByBranch(t.Context(), "feature", "main")
	if err != nil {
		t.Fatalf("GetByBranch: %v", err)
	}
	if got.ID != int64(want.GetNumber()) {
		t.Errorf("ID = %d, want %d", got.ID, want.GetNumber())
	}
	if got.WebURL != want.GetHTMLURL() {
		t.Errorf("WebURL = %q, want %q", got.WebURL, want.GetHTMLURL())
	}
}

func TestGitHubAdapter_WaitForPipelineForwardsTimeout(t *testing.T) {
	mockAPI := mocks.NewGitHubAPIClient()
	mockAPI.WaitForWorkflowsConclusion = "success"

	status, err := newGitHubAdapter(mockAPI).WaitForPipeline(t.Context(), time.Minute)
	if err != nil {
		t.Fatalf("WaitForPipeline: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
	}
}

func TestForgejoAdapter_InitializeAndApprove(t *testing.T) {
	mockAPI := mocks.NewForgejoAPIClient()
	adapter := newForgejoAdapter(mockAPI)

	if err := adapter.Initialize(t.Context(), "https://forgejo.example.com/o/r.git"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	// Forgejo, like GitHub, has no separate approval step.
	if err := adapter.Approve(t.Context(), 1); err != nil {
		t.Errorf("Approve returned %v, want nil", err)
	}
}

func TestForgejoAdapter_MergeForwardsArguments(t *testing.T) {
	mockAPI := mocks.NewForgejoAPIClient()

	if err := newForgejoAdapter(mockAPI).Merge(t.Context(), platform.MergeParams{
		MRID:        77,
		Squash:      true,
		CommitTitle: "Add a feature",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	call := mockAPI.GetLastCall("MergePullRequest")
	if call == nil {
		t.Fatal("adapter did not reach the client")
	}
	if got := call.Args["index"]; got != int64(77) {
		t.Errorf("index = %v, want 77", got)
	}
}

func TestForgejoAdapter_PlatformNameAndTimeout(t *testing.T) {
	adapter := platform.NewForgejoAdapter(mocks.NewForgejoAPIClient(), config.ForgejoConfig{
		PipelineTimeout: "60m",
	}, logger.NoLogger())

	if got := adapter.PlatformName(); got != "Forgejo" {
		t.Errorf("PlatformName = %q, want Forgejo", got)
	}
	if got := adapter.PipelineTimeout(); got != "60m" {
		t.Errorf("PipelineTimeout = %q, want 60m", got)
	}
}

func TestGitHubAdapter_PlatformNameAndTimeout(t *testing.T) {
	adapter := platform.NewGitHubAdapter(mocks.NewGitHubAPIClient(), config.GitHubConfig{
		PipelineTimeout: "20m",
	}, logger.NoLogger())

	if got := adapter.PlatformName(); got != "GitHub" {
		t.Errorf("PlatformName = %q, want GitHub", got)
	}
	if got := adapter.PipelineTimeout(); got != "20m" {
		t.Errorf("PipelineTimeout = %q, want 20m", got)
	}
}

// TestNewProvider_UnsupportedPlatform covers the factory's rejection path; the
// success paths construct real clients and so require tokens.
func TestNewProvider_UnsupportedPlatform(t *testing.T) {
	_, err := platform.NewProvider(t.Context(), "bitbucket", &config.Config{}, logger.NoLogger())
	if err == nil {
		t.Fatal("expected an error for an unsupported platform")
	}
}
