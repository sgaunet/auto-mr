// Package mocks provides mock implementations for testing.
package mocks

import (
	"sync"
	"time"

	glpkg "github.com/sgaunet/auto-mr/pkg/gitlab"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GitLabAPIClient is a mock implementation of gitlab.APIClient with call tracking.
type GitLabAPIClient struct {
	mu    sync.Mutex
	calls []MethodCall

	// Configurable responses
	SetProjectFromURLError         error
	ListLabelsResponse             []*glpkg.Label
	ListLabelsError                error
	CreateMergeRequestResponse     *gitlab.MergeRequest
	CreateMergeRequestError        error
	GetMergeRequestByBranchResponse *gitlab.MergeRequest
	GetMergeRequestByBranchError   error
	WaitForPipelineStatus          string
	WaitForPipelineError           error
	ApproveMergeRequestError       error
	MergeMergeRequestError         error
	GetMergeRequestsByBranchResponse []*gitlab.BasicMergeRequest
	GetMergeRequestsByBranchError  error
}

// NewGitLabAPIClient creates a new mock GitLab API client.
func NewGitLabAPIClient() *GitLabAPIClient {
	return &GitLabAPIClient{
		calls: make([]MethodCall, 0),
	}
}

// SetProjectFromURL implements gitlab.APIClient.
func (m *GitLabAPIClient) SetProjectFromURL(url string) error {
	m.trackCall("SetProjectFromURL", map[string]interface{}{
		"url": url,
	})
	return m.SetProjectFromURLError
}

// ListLabels implements gitlab.APIClient.
func (m *GitLabAPIClient) ListLabels() ([]*glpkg.Label, error) {
	m.trackCall("ListLabels", map[string]interface{}{})
	return m.ListLabelsResponse, m.ListLabelsError
}

// CreateMergeRequest implements gitlab.APIClient.
func (m *GitLabAPIClient) CreateMergeRequest(
	sourceBranch, targetBranch, title, description, assignee, reviewer string,
	labels []string, squash bool,
) (*gitlab.MergeRequest, error) {
	m.trackCall("CreateMergeRequest", map[string]interface{}{
		"sourceBranch": sourceBranch,
		"targetBranch": targetBranch,
		"title":        title,
		"description":  description,
		"assignee":     assignee,
		"reviewer":     reviewer,
		"labels":       labels,
		"squash":       squash,
	})
	return m.CreateMergeRequestResponse, m.CreateMergeRequestError
}

// GetMergeRequestByBranch implements gitlab.APIClient.
func (m *GitLabAPIClient) GetMergeRequestByBranch(sourceBranch, targetBranch string) (*gitlab.MergeRequest, error) {
	m.trackCall("GetMergeRequestByBranch", map[string]interface{}{
		"sourceBranch": sourceBranch,
		"targetBranch": targetBranch,
	})
	return m.GetMergeRequestByBranchResponse, m.GetMergeRequestByBranchError
}

// WaitForPipeline implements gitlab.APIClient.
func (m *GitLabAPIClient) WaitForPipeline(timeout time.Duration) (string, error) {
	m.trackCall("WaitForPipeline", map[string]interface{}{
		"timeout": timeout,
	})
	return m.WaitForPipelineStatus, m.WaitForPipelineError
}

// ApproveMergeRequest implements gitlab.APIClient.
func (m *GitLabAPIClient) ApproveMergeRequest(mrIID int) error {
	m.trackCall("ApproveMergeRequest", map[string]interface{}{
		"mrIID": mrIID,
	})
	return m.ApproveMergeRequestError
}

// MergeMergeRequest implements gitlab.APIClient.
func (m *GitLabAPIClient) MergeMergeRequest(mrIID int, squash bool) error {
	m.trackCall("MergeMergeRequest", map[string]interface{}{
		"mrIID":  mrIID,
		"squash": squash,
	})
	return m.MergeMergeRequestError
}

// GetMergeRequestsByBranch implements gitlab.APIClient.
func (m *GitLabAPIClient) GetMergeRequestsByBranch(sourceBranch string) ([]*gitlab.BasicMergeRequest, error) {
	m.trackCall("GetMergeRequestsByBranch", map[string]interface{}{
		"sourceBranch": sourceBranch,
	})
	return m.GetMergeRequestsByBranchResponse, m.GetMergeRequestsByBranchError
}

// GetCalls returns all tracked method calls.
func (m *GitLabAPIClient) GetCalls() []MethodCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MethodCall{}, m.calls...)
}

// GetCallCount returns the number of times a method was called.
func (m *GitLabAPIClient) GetCallCount(method string) int {
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

// GetLastCall returns the last call to the specified method, or nil if not called.
func (m *GitLabAPIClient) GetLastCall(method string) *MethodCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.calls) - 1; i >= 0; i-- {
		if m.calls[i].Method == method {
			return &m.calls[i]
		}
	}
	return nil
}

// Reset clears all tracked calls.
func (m *GitLabAPIClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make([]MethodCall, 0)
}

// trackCall records a method call with its arguments.
func (m *GitLabAPIClient) trackCall(method string, args map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, MethodCall{
		Method: method,
		Args:   args,
	})
}

// Ensure GitLabAPIClient implements gitlab.APIClient interface.
var _ glpkg.APIClient = (*GitLabAPIClient)(nil)
