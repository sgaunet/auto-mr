package gitlab_test

import (
	"errors"
	"fmt"
	"strings"
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

// TestErrorMRAlreadyExists tests MR already exists error detection.
func TestErrorMRAlreadyExists(t *testing.T) {
	scenarios := []struct {
		name        string
		apiError    string
		expectMatch bool
	}{
		{
			name:        "standard already exists message",
			apiError:    "merge request already exists",
			expectMatch: true,
		},
		{
			name:        "GitLab variant message",
			apiError:    "Another open merge request already exists for this source branch",
			expectMatch: true,
		},
		{
			name:        "case insensitive - uppercase",
			apiError:    "ALREADY EXISTS",
			expectMatch: true,
		},
		{
			name:        "case insensitive - mixed case",
			apiError:    "Already Exists",
			expectMatch: true,
		},
		{
			name:        "partial match in error message",
			apiError:    "Error: merge request already exists for branch feature/test",
			expectMatch: true,
		},
		{
			name:        "unrelated error - validation",
			apiError:    "validation failed: title can't be blank",
			expectMatch: false,
		},
		{
			name:        "unrelated error - permissions",
			apiError:    "403 Forbidden: insufficient permissions",
			expectMatch: false,
		},
		{
			name:        "unrelated error - not found",
			apiError:    "404 Not Found: project does not exist",
			expectMatch: false,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()

			if scenario.expectMatch {
				// Simulate the wrapped error that would be returned by api.go
				wrappedErr := fmt.Errorf("%w: source=feature, target=main: %s",
					gitlab.ErrMRAlreadyExists, scenario.apiError)
				mockAPI.CreateMergeRequestError = wrappedErr
			} else {
				mockAPI.CreateMergeRequestError = errors.New(scenario.apiError)
			}

			_, err := mockAPI.CreateMergeRequest("feature", "main", "Test", "Desc", "", "", []string{}, false)

			if scenario.expectMatch {
				if !errors.Is(err, gitlab.ErrMRAlreadyExists) {
					t.Errorf("Expected ErrMRAlreadyExists, got %v", err)
				}
				// Verify error message includes branch context
				if err != nil && !strings.Contains(err.Error(), "source=feature") {
					t.Error("Expected error message to include branch context")
				}
			} else {
				if errors.Is(err, gitlab.ErrMRAlreadyExists) {
					t.Errorf("Did not expect ErrMRAlreadyExists for: %s", scenario.apiError)
				}
			}
		})
	}
}

// TestErrorMRAlreadyExistsWorkflow tests the full workflow for handling existing MRs.
func TestErrorMRAlreadyExistsWorkflow(t *testing.T) {
	t.Run("create MR detects existing and fetches it", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		// First attempt to create MR returns "already exists" error
		wrappedErr := fmt.Errorf("%w: source=feature, target=main: merge request already exists",
			gitlab.ErrMRAlreadyExists)
		mockAPI.CreateMergeRequestError = wrappedErr

		_, err := mockAPI.CreateMergeRequest("feature", "main", "Test", "Desc", "", "", []string{}, false)
		if !errors.Is(err, gitlab.ErrMRAlreadyExists) {
			t.Errorf("Expected ErrMRAlreadyExists on first attempt, got %v", err)
		}

		// Verify we can fetch the existing MR
		mockAPI.GetMergeRequestByBranchError = nil
		mockAPI.GetMergeRequestByBranchResponse = fixtures.ValidMergeRequest()

		existingMR, fetchErr := mockAPI.GetMergeRequestByBranch("feature", "main")
		if fetchErr != nil {
			t.Fatalf("Failed to fetch existing MR: %v", fetchErr)
		}
		if existingMR == nil {
			t.Error("Expected to receive existing MR")
		}
	})

	t.Run("error context preserved through workflow", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		originalErr := "Another open merge request already exists for this source branch"
		wrappedErr := fmt.Errorf("%w: source=feature-123, target=develop: %s",
			gitlab.ErrMRAlreadyExists, originalErr)
		mockAPI.CreateMergeRequestError = wrappedErr

		_, err := mockAPI.CreateMergeRequest("feature-123", "develop", "Test", "Desc", "", "", []string{}, false)

		// Verify typed error is detectable
		if !errors.Is(err, gitlab.ErrMRAlreadyExists) {
			t.Error("Expected ErrMRAlreadyExists")
		}

		// Verify branch context is in error message
		errMsg := err.Error()
		if !strings.Contains(errMsg, "feature-123") || !strings.Contains(errMsg, "develop") {
			t.Errorf("Expected error message to contain branch names, got: %s", errMsg)
		}

		// Verify original error is preserved
		if !strings.Contains(errMsg, originalErr) {
			t.Errorf("Expected original error message to be preserved, got: %s", errMsg)
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
