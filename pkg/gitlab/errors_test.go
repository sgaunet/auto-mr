package gitlab_test

import (
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
)

// TestErrorTokenRequired tests token requirement errors.
func TestErrorTokenRequired(t *testing.T) {
	t.Run("missing token error", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.ListLabelsError = gitlab.ErrTokenRequired

		_, err := mockAPI.ListLabels()
		if err == nil || err != gitlab.ErrTokenRequired {
			t.Error("Expected ErrTokenRequired")
		}
	})
}

// TestErrorInvalidURLFormat tests URL format validation errors.
func TestErrorInvalidURLFormat(t *testing.T) {
	urls := []string{"", "not-a-url", "gitlab.com/owner", "https://gitlab.com/", "ftp://gitlab.com/owner/project"}

	for _, url := range urls {
		t.Run("invalid URL: "+url, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()
			mockAPI.SetProjectFromURLError = gitlab.ErrInvalidURLFormat

			err := mockAPI.SetProjectFromURL(url)
			if err == nil {
				t.Error("Expected error for invalid URL")
			}
		})
	}
}

// TestErrorPipelineTimeout tests pipeline timeout errors.
func TestErrorPipelineTimeout(t *testing.T) {
	t.Run("timeout error on pipeline wait", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.WaitForPipelineError = gitlab.ErrPipelineTimeout

		_, err := mockAPI.WaitForPipeline(1 * time.Millisecond)
		if err == nil || err != gitlab.ErrPipelineTimeout {
			t.Error("Expected ErrPipelineTimeout")
		}
	})
}

// TestErrorMRNotFound tests MR not found errors.
func TestErrorMRNotFound(t *testing.T) {
	t.Run("MR not found for branch", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.GetMergeRequestByBranchError = gitlab.ErrMRNotFound

		_, err := mockAPI.GetMergeRequestByBranch("nonexistent", "main")
		if err == nil || err != gitlab.ErrMRNotFound {
			t.Error("Expected ErrMRNotFound")
		}
	})
}

// TestErrorAPIFailures tests various API failure scenarios.
func TestErrorAPIFailures(t *testing.T) {
	scenarios := []struct {
		name      string
		setupMock func(*mocks.GitLabAPIClient)
		testFunc  func(*mocks.GitLabAPIClient) error
	}{
		{
			name: "ListLabels API failure",
			setupMock: func(m *mocks.GitLabAPIClient) {
				m.ListLabelsError = gitlab.ErrTokenRequired
			},
			testFunc: func(m *mocks.GitLabAPIClient) error {
				_, err := m.ListLabels()
				return err
			},
		},
		{
			name: "CreateMergeRequest API failure",
			setupMock: func(m *mocks.GitLabAPIClient) {
				m.CreateMergeRequestError = gitlab.ErrInvalidURLFormat
			},
			testFunc: func(m *mocks.GitLabAPIClient) error {
				_, err := m.CreateMergeRequest("feature", "main", "Test", "Desc", "", "", []string{}, false)
				return err
			},
		},
		{
			name: "MergeMergeRequest API failure",
			setupMock: func(m *mocks.GitLabAPIClient) {
				m.MergeMergeRequestError = gitlab.ErrMRNotFound
			},
			testFunc: func(m *mocks.GitLabAPIClient) error {
				return m.MergeMergeRequest(123, false, "Test commit")
			},
		},
		{
			name: "ApproveMergeRequest API failure",
			setupMock: func(m *mocks.GitLabAPIClient) {
				m.ApproveMergeRequestError = gitlab.ErrTokenRequired
			},
			testFunc: func(m *mocks.GitLabAPIClient) error {
				return m.ApproveMergeRequest(123)
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()
			scenario.setupMock(mockAPI)

			err := scenario.testFunc(mockAPI)
			if err == nil {
				t.Error("Expected API failure error")
			}
		})
	}
}

// TestErrorRecovery tests error recovery and retry logic.
func TestErrorRecovery(t *testing.T) {
	t.Run("retry after transient error", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		// First attempt - fails
		mockAPI.CreateMergeRequestError = gitlab.ErrTokenRequired
		_, err := mockAPI.CreateMergeRequest("feature", "main", "Test", "Desc", "", "", []string{}, false)
		if err == nil {
			t.Error("Expected first attempt to fail")
		}

		// Second attempt - succeeds
		mockAPI.CreateMergeRequestError = nil
		mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
		_, err = mockAPI.CreateMergeRequest("feature", "main", "Test", "Desc", "", "", []string{}, false)
		if err != nil {
			t.Error("Expected second attempt to succeed")
		}
	})
}
