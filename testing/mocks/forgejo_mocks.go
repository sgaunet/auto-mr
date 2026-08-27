package mocks

import (
	"context"
	"slices"
	"sync"
	"time"

	"code.gitea.io/sdk/gitea"
	fjpkg "github.com/sgaunet/auto-mr/pkg/forgejo"
)

// ForgejoAPIClient is a mock implementation of forgejo.APIClient with call tracking.
type ForgejoAPIClient struct {
	mu    sync.Mutex
	calls []MethodCall

	// Configurable responses
	SetRepositoryFromURLError   error
	ListLabelsResponse          []fjpkg.Label
	ListLabelsError             error
	CreatePullRequestResponse   *gitea.PullRequest
	CreatePullRequestError      error
	GetPullRequestByBranchResp  *gitea.PullRequest
	GetPullRequestByBranchError error
	WaitForPipelineStatus       string
	WaitForPipelineError        error
	MergePullRequestError       error
}

// NewForgejoAPIClient creates a new mock Forgejo API client.
func NewForgejoAPIClient() *ForgejoAPIClient {
	return &ForgejoAPIClient{
		calls: make([]MethodCall, 0),
	}
}

// SetRepositoryFromURL implements forgejo.APIClient.
func (m *ForgejoAPIClient) SetRepositoryFromURL(_ context.Context, url string) error {
	m.trackCall("SetRepositoryFromURL", map[string]any{
		argURL: url,
	})
	return m.SetRepositoryFromURLError
}

// ListLabels implements forgejo.APIClient.
func (m *ForgejoAPIClient) ListLabels(_ context.Context) ([]fjpkg.Label, error) {
	m.trackCall("ListLabels", nil)
	if m.ListLabelsError != nil {
		return nil, m.ListLabelsError
	}
	return m.ListLabelsResponse, nil
}

// CreatePullRequest implements forgejo.APIClient.
func (m *ForgejoAPIClient) CreatePullRequest(
	_ context.Context,
	head, base, title, body, assignee, reviewer string,
	labels []string,
) (*gitea.PullRequest, error) {
	m.trackCall("CreatePullRequest", map[string]any{
		argHead:    head,
		argBase:    base,
		argTitle:   title,
		argBody:    body,
		"assignee": assignee,
		"reviewer": reviewer,
		argLabels:  labels,
	})
	if m.CreatePullRequestError != nil {
		return nil, m.CreatePullRequestError
	}
	return m.CreatePullRequestResponse, nil
}

// GetPullRequestByBranch implements forgejo.APIClient.
func (m *ForgejoAPIClient) GetPullRequestByBranch(
	_ context.Context, head, base string,
) (*gitea.PullRequest, error) {
	m.trackCall("GetPullRequestByBranch", map[string]any{
		argHead: head,
		argBase: base,
	})
	if m.GetPullRequestByBranchError != nil {
		return nil, m.GetPullRequestByBranchError
	}
	return m.GetPullRequestByBranchResp, nil
}

// WaitForPipeline implements forgejo.APIClient.
func (m *ForgejoAPIClient) WaitForPipeline(_ context.Context, timeout time.Duration) (string, error) {
	m.trackCall("WaitForPipeline", map[string]any{
		argTimeout: timeout,
	})
	if m.WaitForPipelineError != nil {
		return "", m.WaitForPipelineError
	}
	return m.WaitForPipelineStatus, nil
}

// MergePullRequest implements forgejo.APIClient.
func (m *ForgejoAPIClient) MergePullRequest(
	_ context.Context, index int64, squash bool, commitTitle string,
) error {
	m.trackCall("MergePullRequest", map[string]any{
		"index":        index,
		argSquash:      squash,
		argCommitTitle: commitTitle,
	})
	return m.MergePullRequestError
}

// GetCalls returns all recorded method calls.
func (m *ForgejoAPIClient) GetCalls() []MethodCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MethodCall(nil), m.calls...)
}

// GetCallCount returns how many times the named method was called.
func (m *ForgejoAPIClient) GetCallCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, call := range m.calls {
		if call.Method == method {
			count++
		}
	}
	return count
}

// GetLastCall returns the most recent call to the named method, or nil.
func (m *ForgejoAPIClient) GetLastCall(method string) *MethodCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, call := range slices.Backward(m.calls) {
		if call.Method == method {
			return &call
		}
	}
	return nil
}

// Reset clears all recorded calls.
func (m *ForgejoAPIClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make([]MethodCall, 0)
}

func (m *ForgejoAPIClient) trackCall(method string, args map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, MethodCall{Method: method, Args: args})
}

// Ensure ForgejoAPIClient implements forgejo.APIClient interface.
var _ fjpkg.APIClient = (*ForgejoAPIClient)(nil)
