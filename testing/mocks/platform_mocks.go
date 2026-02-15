package mocks

import (
	"sync"
	"time"

	"github.com/sgaunet/auto-mr/pkg/platform"
)

// PlatformProvider is a mock implementation of platform.Provider with call tracking.
type PlatformProvider struct {
	mu    sync.Mutex
	calls []MethodCall

	// Configurable responses
	InitializeError       error
	ListLabelsResponse    []platform.Label
	ListLabelsError       error
	CreateResponse        *platform.MergeRequest
	CreateError           error
	GetByBranchResponse   *platform.MergeRequest
	GetByBranchError      error
	WaitForPipelineStatus string
	WaitForPipelineError  error
	ApproveError          error
	MergeError            error
	PlatformNameValue     string
	PipelineTimeoutValue  string
}

// NewPlatformProvider creates a new mock platform provider.
func NewPlatformProvider() *PlatformProvider {
	return &PlatformProvider{
		calls:             make([]MethodCall, 0),
		PlatformNameValue: "MockPlatform",
	}
}

// Initialize implements platform.Provider.
func (m *PlatformProvider) Initialize(remoteURL string) error {
	m.trackCall("Initialize", map[string]any{
		"remoteURL": remoteURL,
	})
	return m.InitializeError
}

// ListLabels implements platform.Provider.
func (m *PlatformProvider) ListLabels() ([]platform.Label, error) {
	m.trackCall("ListLabels", map[string]any{})
	return m.ListLabelsResponse, m.ListLabelsError
}

// Create implements platform.Provider.
func (m *PlatformProvider) Create(params platform.CreateParams) (*platform.MergeRequest, error) {
	m.trackCall("Create", map[string]any{
		"sourceBranch": params.SourceBranch,
		"targetBranch": params.TargetBranch,
		"title":        params.Title,
		"body":         params.Body,
		"labels":       params.Labels,
		"squash":       params.Squash,
	})
	return m.CreateResponse, m.CreateError
}

// GetByBranch implements platform.Provider.
func (m *PlatformProvider) GetByBranch(sourceBranch, targetBranch string) (*platform.MergeRequest, error) {
	m.trackCall("GetByBranch", map[string]any{
		"sourceBranch": sourceBranch,
		"targetBranch": targetBranch,
	})
	return m.GetByBranchResponse, m.GetByBranchError
}

// WaitForPipeline implements platform.Provider.
func (m *PlatformProvider) WaitForPipeline(timeout time.Duration) (string, error) {
	m.trackCall("WaitForPipeline", map[string]any{
		"timeout": timeout,
	})
	return m.WaitForPipelineStatus, m.WaitForPipelineError
}

// Approve implements platform.Provider.
func (m *PlatformProvider) Approve(mrID int64) error {
	m.trackCall("Approve", map[string]any{
		"mrID": mrID,
	})
	return m.ApproveError
}

// Merge implements platform.Provider.
func (m *PlatformProvider) Merge(params platform.MergeParams) error {
	m.trackCall("Merge", map[string]any{
		"mrID":         params.MRID,
		"squash":       params.Squash,
		"commitTitle":  params.CommitTitle,
		"sourceBranch": params.SourceBranch,
	})
	return m.MergeError
}

// PlatformName implements platform.Provider.
func (m *PlatformProvider) PlatformName() string {
	return m.PlatformNameValue
}

// PipelineTimeout implements platform.Provider.
func (m *PlatformProvider) PipelineTimeout() string {
	return m.PipelineTimeoutValue
}

// GetCalls returns all tracked method calls.
func (m *PlatformProvider) GetCalls() []MethodCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MethodCall{}, m.calls...)
}

// GetCallCount returns the number of times a method was called.
func (m *PlatformProvider) GetCallCount(method string) int {
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
func (m *PlatformProvider) GetLastCall(method string) *MethodCall {
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
func (m *PlatformProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make([]MethodCall, 0)
}

// trackCall records a method call with its arguments.
func (m *PlatformProvider) trackCall(method string, args map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, MethodCall{
		Method: method,
		Args:   args,
	})
}

// Ensure PlatformProvider implements platform.Provider interface.
var _ platform.Provider = (*PlatformProvider)(nil)
