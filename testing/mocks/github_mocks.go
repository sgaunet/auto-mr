package mocks

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v69/github"
	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/bullets"
)

// GitHubAPIClient is a mock implementation of github.APIClient with call tracking.
type GitHubAPIClient struct {
	mu    sync.Mutex
	calls []MethodCall

	// Configurable responses
	SetRepositoryFromURLError      error
	ListLabelsResponse             []*ghpkg.Label
	ListLabelsError                error
	CreatePullRequestResponse      *github.PullRequest
	CreatePullRequestError         error
	GetPullRequestByBranchResponse *github.PullRequest
	GetPullRequestByBranchError    error
	WaitForWorkflowsConclusion     string
	WaitForWorkflowsError          error
	MergePullRequestError          error
	GetPullRequestsByHeadResponse  []*github.PullRequest
	GetPullRequestsByHeadError     error
	DeleteBranchError              error
}

// MethodCall represents a tracked method call with its parameters.
type MethodCall struct {
	Method string
	Args   map[string]any
}

// NewGitHubAPIClient creates a new mock GitHub API client.
func NewGitHubAPIClient() *GitHubAPIClient {
	return &GitHubAPIClient{
		calls: make([]MethodCall, 0),
	}
}

// SetRepositoryFromURL implements github.APIClient.
func (m *GitHubAPIClient) SetRepositoryFromURL(url string) error {
	m.trackCall("SetRepositoryFromURL", map[string]any{
		"url": url,
	})
	return m.SetRepositoryFromURLError
}

// ListLabels implements github.APIClient.
func (m *GitHubAPIClient) ListLabels() ([]*ghpkg.Label, error) {
	m.trackCall("ListLabels", map[string]any{})
	return m.ListLabelsResponse, m.ListLabelsError
}

// CreatePullRequest implements github.APIClient.
func (m *GitHubAPIClient) CreatePullRequest(
	head, base, title, body string,
	assignees, reviewers, labels []string,
) (*github.PullRequest, error) {
	m.trackCall("CreatePullRequest", map[string]any{
		"head":      head,
		"base":      base,
		"title":     title,
		"body":      body,
		"assignees": assignees,
		"reviewers": reviewers,
		"labels":    labels,
	})
	return m.CreatePullRequestResponse, m.CreatePullRequestError
}

// GetPullRequestByBranch implements github.APIClient.
func (m *GitHubAPIClient) GetPullRequestByBranch(head, base string) (*github.PullRequest, error) {
	m.trackCall("GetPullRequestByBranch", map[string]any{
		"head": head,
		"base": base,
	})
	return m.GetPullRequestByBranchResponse, m.GetPullRequestByBranchError
}

// WaitForWorkflows implements github.APIClient.
func (m *GitHubAPIClient) WaitForWorkflows(timeout time.Duration) (string, error) {
	m.trackCall("WaitForWorkflows", map[string]any{
		"timeout": timeout,
	})
	return m.WaitForWorkflowsConclusion, m.WaitForWorkflowsError
}

// MergePullRequest implements github.APIClient.
func (m *GitHubAPIClient) MergePullRequest(prNumber int, mergeMethod, commitTitle string) error {
	m.trackCall("MergePullRequest", map[string]any{
		"prNumber":    prNumber,
		"mergeMethod": mergeMethod,
		"commitTitle": commitTitle,
	})
	return m.MergePullRequestError
}

// GetPullRequestsByHead implements github.APIClient.
func (m *GitHubAPIClient) GetPullRequestsByHead(head string) ([]*github.PullRequest, error) {
	m.trackCall("GetPullRequestsByHead", map[string]any{
		"head": head,
	})
	return m.GetPullRequestsByHeadResponse, m.GetPullRequestsByHeadError
}

// DeleteBranch implements github.APIClient.
func (m *GitHubAPIClient) DeleteBranch(branch string) error {
	m.trackCall("DeleteBranch", map[string]any{
		"branch": branch,
	})
	return m.DeleteBranchError
}

// GetCalls returns all tracked method calls.
func (m *GitHubAPIClient) GetCalls() []MethodCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MethodCall{}, m.calls...)
}

// GetCallCount returns the number of times a method was called.
func (m *GitHubAPIClient) GetCallCount(method string) int {
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
func (m *GitHubAPIClient) GetLastCall(method string) *MethodCall {
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
func (m *GitHubAPIClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make([]MethodCall, 0)
}

// trackCall records a method call with its arguments.
func (m *GitHubAPIClient) trackCall(method string, args map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, MethodCall{
		Method: method,
		Args:   args,
	})
}

// Ensure GitHubAPIClient implements github.APIClient interface.
var _ ghpkg.APIClient = (*GitHubAPIClient)(nil)

// MockDisplayRenderer is a mock implementation of DisplayRenderer for testing.
type MockDisplayRenderer struct {
	mu       sync.Mutex
	messages []DisplayMessage
}

// DisplayMessage represents a logged message with its level.
type DisplayMessage struct {
	Level   string
	Message string
}

// NewMockDisplayRenderer creates a new mock display renderer.
func NewMockDisplayRenderer() *MockDisplayRenderer {
	return &MockDisplayRenderer{
		messages: make([]DisplayMessage, 0),
	}
}

// Info implements DisplayRenderer.
func (m *MockDisplayRenderer) Info(message string) {
	m.trackMessage("info", message)
}

// Debug implements DisplayRenderer.
func (m *MockDisplayRenderer) Debug(message string) {
	m.trackMessage("debug", message)
}

// Error implements DisplayRenderer.
func (m *MockDisplayRenderer) Error(message string) {
	m.trackMessage("error", message)
}

// Success implements DisplayRenderer.
func (m *MockDisplayRenderer) Success(message string) {
	m.trackMessage("success", message)
}

// InfoHandle implements DisplayRenderer.
func (m *MockDisplayRenderer) InfoHandle(message string) *bullets.BulletHandle {
	m.trackMessage("info_handle", message)
	// Return a mock handle - in real tests you might want to track handle operations too
	return &bullets.BulletHandle{}
}

// SpinnerCircle implements DisplayRenderer.
func (m *MockDisplayRenderer) SpinnerCircle(message string) *bullets.Spinner {
	m.trackMessage("spinner", message)
	// Return a mock spinner - in real tests you might want to track spinner operations too
	return &bullets.Spinner{}
}

// IncreasePadding implements DisplayRenderer.
func (m *MockDisplayRenderer) IncreasePadding() {
	m.trackMessage("increase_padding", "")
}

// DecreasePadding implements DisplayRenderer.
func (m *MockDisplayRenderer) DecreasePadding() {
	m.trackMessage("decrease_padding", "")
}

// GetMessages returns all tracked messages.
func (m *MockDisplayRenderer) GetMessages() []DisplayMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DisplayMessage{}, m.messages...)
}

// GetMessagesByLevel returns all messages of a specific level.
func (m *MockDisplayRenderer) GetMessagesByLevel(level string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []string
	for _, msg := range m.messages {
		if msg.Level == level {
			result = append(result, msg.Message)
		}
	}
	return result
}

// Reset clears all tracked messages.
func (m *MockDisplayRenderer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = make([]DisplayMessage, 0)
}

// String returns a formatted representation of all messages for debugging.
func (m *MockDisplayRenderer) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result strings.Builder
	for i, msg := range m.messages {
		result.WriteString(fmt.Sprintf("[%d] %s: %s\n", i, msg.Level, msg.Message))
	}
	return result.String()
}

// trackMessage records a display message.
func (m *MockDisplayRenderer) trackMessage(level, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, DisplayMessage{
		Level:   level,
		Message: message,
	})
}
