package mocks

import "github.com/sgaunet/auto-mr/pkg/commits"

// MockCommitRetriever is a mock implementation of commits.CommitRetriever for testing.
type MockCommitRetriever struct {
	GetCommitsResponse []commits.Commit
	GetCommitsError    error
	CallHistory        []string
	CallCount          map[string]int
}

// NewMockCommitRetriever creates a new mock commit retriever.
func NewMockCommitRetriever() *MockCommitRetriever {
	return &MockCommitRetriever{
		CallHistory: []string{},
		CallCount:   make(map[string]int),
	}
}

// GetCommits implements commits.CommitRetriever interface.
func (m *MockCommitRetriever) GetCommits(branch string) ([]commits.Commit, error) {
	m.CallHistory = append(m.CallHistory, "GetCommits:"+branch)
	m.CallCount["GetCommits"]++
	return m.GetCommitsResponse, m.GetCommitsError
}

// GetLastCall returns the last method call recorded.
func (m *MockCommitRetriever) GetLastCall() string {
	if len(m.CallHistory) == 0 {
		return ""
	}
	return m.CallHistory[len(m.CallHistory)-1]
}

// GetCallCountFor returns the number of times a method was called.
func (m *MockCommitRetriever) GetCallCountFor(method string) int {
	return m.CallCount[method]
}

// MockMessageSelector is a mock implementation of commits.MessageSelector for testing.
type MockMessageSelector struct {
	GetMessageForMRResponse commits.MessageSelection
	GetMessageForMRError    error
	CallHistory             []string
	CallCount               map[string]int
}

// NewMockMessageSelector creates a new mock message selector.
func NewMockMessageSelector() *MockMessageSelector {
	return &MockMessageSelector{
		CallHistory: []string{},
		CallCount:   make(map[string]int),
	}
}

// GetMessageForMR implements commits.MessageSelector interface.
func (m *MockMessageSelector) GetMessageForMR(cmts []commits.Commit, msgFlagValue string) (commits.MessageSelection, error) {
	m.CallHistory = append(m.CallHistory, "GetMessageForMR")
	m.CallCount["GetMessageForMR"]++
	return m.GetMessageForMRResponse, m.GetMessageForMRError
}

// GetLastCall returns the last method call recorded.
func (m *MockMessageSelector) GetLastCall() string {
	if len(m.CallHistory) == 0 {
		return ""
	}
	return m.CallHistory[len(m.CallHistory)-1]
}

// GetCallCountFor returns the number of times a method was called.
func (m *MockMessageSelector) GetCallCountFor(method string) int {
	return m.CallCount[method]
}

// MockSelectionRenderer is a mock implementation of commits.SelectionRenderer for testing.
type MockSelectionRenderer struct {
	DisplaySelectionPromptResponse int
	DisplaySelectionPromptError    error
	CallHistory                    []string
	CallCount                      map[string]int
}

// NewMockSelectionRenderer creates a new mock selection renderer.
func NewMockSelectionRenderer() *MockSelectionRenderer {
	return &MockSelectionRenderer{
		CallHistory: []string{},
		CallCount:   make(map[string]int),
	}
}

// DisplaySelectionPrompt implements commits.SelectionRenderer interface.
func (m *MockSelectionRenderer) DisplaySelectionPrompt(cmts []commits.Commit) (int, error) {
	m.CallHistory = append(m.CallHistory, "DisplaySelectionPrompt")
	m.CallCount["DisplaySelectionPrompt"]++
	return m.DisplaySelectionPromptResponse, m.DisplaySelectionPromptError
}

// GetLastCall returns the last method call recorded.
func (m *MockSelectionRenderer) GetLastCall() string {
	if len(m.CallHistory) == 0 {
		return ""
	}
	return m.CallHistory[len(m.CallHistory)-1]
}

// GetCallCountFor returns the number of times a method was called.
func (m *MockSelectionRenderer) GetCallCountFor(method string) int {
	return m.CallCount[method]
}
