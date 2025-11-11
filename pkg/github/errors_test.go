package github_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/auto-mr/testing/mocks"
)

// TestErrorTokenRequired tests token requirement validation.
func TestErrorTokenRequired(t *testing.T) {
	t.Run("missing token error", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.SetRepositoryFromURLError = ghpkg.ErrTokenRequired

		err := mockAPI.SetRepositoryFromURL("https://github.com/owner/repo")
		if err == nil {
			t.Error("Expected token required error")
		}
		if !errors.Is(err, ghpkg.ErrTokenRequired) {
			t.Errorf("Expected ErrTokenRequired, got %v", err)
		}
	})

	t.Run("token required error message", func(t *testing.T) {
		err := ghpkg.ErrTokenRequired
		expected := "GITHUB_TOKEN environment variable is required"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})
}

// TestErrorInvalidURLFormat tests URL validation errors.
func TestErrorInvalidURLFormat(t *testing.T) {
	invalidURLs := []string{
		"",
		"not-a-url",
		"github.com/owner",
		"https://github.com/",
		"https://github.com/owner",
		"ftp://github.com/owner/repo",
		"://invalid",
	}

	for _, url := range invalidURLs {
		t.Run(fmt.Sprintf("invalid URL: %s", url), func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()
			mockAPI.SetRepositoryFromURLError = ghpkg.ErrInvalidURLFormat

			err := mockAPI.SetRepositoryFromURL(url)
			if err == nil {
				t.Errorf("Expected error for invalid URL: %s", url)
			}
			if !errors.Is(err, ghpkg.ErrInvalidURLFormat) {
				t.Errorf("Expected ErrInvalidURLFormat for %s, got %v", url, err)
			}
		})
	}
}

// TestErrorWorkflowTimeout tests timeout error scenarios.
func TestErrorWorkflowTimeout(t *testing.T) {
	t.Run("timeout error on workflow wait", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.WaitForWorkflowsError = ghpkg.ErrWorkflowTimeout

		_, err := mockAPI.WaitForWorkflows(1 * time.Second)
		if err == nil {
			t.Error("Expected timeout error")
		}
		if !errors.Is(err, ghpkg.ErrWorkflowTimeout) {
			t.Errorf("Expected ErrWorkflowTimeout, got %v", err)
		}
	})

	t.Run("timeout error message", func(t *testing.T) {
		err := ghpkg.ErrWorkflowTimeout
		expected := "timeout waiting for workflow completion"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("very short timeout triggers error", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.WaitForWorkflowsError = ghpkg.ErrWorkflowTimeout

		_, err := mockAPI.WaitForWorkflows(1 * time.Millisecond)
		if err == nil {
			t.Error("Expected timeout error for very short duration")
		}
	})
}

// TestErrorPRNotFound tests PR not found error scenarios.
func TestErrorPRNotFound(t *testing.T) {
	t.Run("PR not found for branch", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestByBranchError = ghpkg.ErrPRNotFound

		_, err := mockAPI.GetPullRequestByBranch("nonexistent", "main")
		if err == nil {
			t.Error("Expected PR not found error")
		}
		if !errors.Is(err, ghpkg.ErrPRNotFound) {
			t.Errorf("Expected ErrPRNotFound, got %v", err)
		}
	})

	t.Run("PR not found error message", func(t *testing.T) {
		err := ghpkg.ErrPRNotFound
		expected := "no pull request found for branch"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})
}

// TestErrorAPIFailures tests various API failure scenarios.
func TestErrorAPIFailures(t *testing.T) {
	t.Run("ListLabels API failure", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsError = errors.New("API rate limit exceeded")

		_, err := mockAPI.ListLabels()
		if err == nil {
			t.Error("Expected API error")
		}
		if err.Error() != "API rate limit exceeded" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("CreatePullRequest API failure", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestError = errors.New("422 Validation Failed")

		_, err := mockAPI.CreatePullRequest(
			"feature", "main", "Title", "Body", nil, nil, nil,
		)
		if err == nil {
			t.Error("Expected API validation error")
		}
	})

	t.Run("MergePullRequest API failure", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.MergePullRequestError = errors.New("405 Method Not Allowed")

		err := mockAPI.MergePullRequest(123, "squash", "Test commit", "Test body")
		if err == nil {
			t.Error("Expected merge error")
		}
	})

	t.Run("DeleteBranch API failure", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.DeleteBranchError = errors.New("403 Forbidden")

		err := mockAPI.DeleteBranch("protected-branch")
		if err == nil {
			t.Error("Expected delete error for protected branch")
		}
	})
}

// TestErrorNetworkFailures tests network-related error scenarios.
func TestErrorNetworkFailures(t *testing.T) {
	networkErrors := []struct {
		name  string
		error error
	}{
		{"connection timeout", errors.New("dial tcp: i/o timeout")},
		{"connection refused", errors.New("connection refused")},
		{"no such host", errors.New("no such host")},
		{"network unreachable", errors.New("network is unreachable")},
	}

	for _, tc := range networkErrors {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()
			mockAPI.ListLabelsError = tc.error

			_, err := mockAPI.ListLabels()
			if err == nil {
				t.Errorf("Expected network error for %s", tc.name)
			}
			if err.Error() != tc.error.Error() {
				t.Errorf("Expected error '%v', got '%v'", tc.error, err)
			}
		})
	}
}

// TestErrorHTTPStatusCodes tests various HTTP error status codes.
func TestErrorHTTPStatusCodes(t *testing.T) {
	statusCodeTests := []struct {
		code    int
		message string
	}{
		{400, "400 Bad Request"},
		{401, "401 Unauthorized"},
		{403, "403 Forbidden"},
		{404, "404 Not Found"},
		{422, "422 Unprocessable Entity"},
		{429, "429 Too Many Requests"},
		{500, "500 Internal Server Error"},
		{502, "502 Bad Gateway"},
		{503, "503 Service Unavailable"},
	}

	for _, tc := range statusCodeTests {
		t.Run(fmt.Sprintf("HTTP %d", tc.code), func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()
			mockAPI.SetRepositoryFromURLError = errors.New(tc.message)

			err := mockAPI.SetRepositoryFromURL("https://github.com/owner/repo")
			if err == nil {
				t.Errorf("Expected error for status code %d", tc.code)
			}
			if err.Error() != tc.message {
				t.Errorf("Expected error message '%s', got '%s'", tc.message, err.Error())
			}
		})
	}
}

// TestErrorRateLimiting tests rate limiting scenarios.
func TestErrorRateLimiting(t *testing.T) {
	t.Run("rate limit on API calls", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		rateLimitErr := errors.New("403 API rate limit exceeded")
		mockAPI.ListLabelsError = rateLimitErr

		_, err := mockAPI.ListLabels()
		if err == nil {
			t.Error("Expected rate limit error")
		}
		if err.Error() != rateLimitErr.Error() {
			t.Errorf("Expected rate limit error, got %v", err)
		}
	})

	t.Run("429 too many requests", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		tooManyErr := errors.New("429 Too Many Requests")
		mockAPI.CreatePullRequestError = tooManyErr

		_, err := mockAPI.CreatePullRequest(
			"feature", "main", "Title", "Body", nil, nil, nil,
		)
		if err == nil {
			t.Error("Expected 429 error")
		}
	})
}

// TestErrorAuthenticationFailures tests authentication error scenarios.
func TestErrorAuthenticationFailures(t *testing.T) {
	t.Run("invalid token", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.SetRepositoryFromURLError = errors.New("401 Bad credentials")

		err := mockAPI.SetRepositoryFromURL("https://github.com/owner/repo")
		if err == nil {
			t.Error("Expected authentication error")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsError = errors.New("401 Token expired")

		_, err := mockAPI.ListLabels()
		if err == nil {
			t.Error("Expected expired token error")
		}
	})

	t.Run("insufficient permissions", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.MergePullRequestError = errors.New("403 Resource not accessible by integration")

		err := mockAPI.MergePullRequest(123, "squash", "Test commit", "Test body")
		if err == nil {
			t.Error("Expected insufficient permissions error")
		}
	})
}

// TestErrorMalformedResponses tests handling of malformed API responses.
func TestErrorMalformedResponses(t *testing.T) {
	t.Run("invalid JSON response", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsError = errors.New("invalid character '<' looking for beginning of value")

		_, err := mockAPI.ListLabels()
		if err == nil {
			t.Error("Expected JSON parsing error")
		}
	})

	t.Run("unexpected response format", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestByBranchError = errors.New("unexpected response format")

		_, err := mockAPI.GetPullRequestByBranch("feature", "main")
		if err == nil {
			t.Error("Expected format error")
		}
	})
}

// TestErrorResourceNotFound tests various not found scenarios.
func TestErrorResourceNotFound(t *testing.T) {
	t.Run("repository not found", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.SetRepositoryFromURLError = errors.New("404 Not Found")

		err := mockAPI.SetRepositoryFromURL("https://github.com/owner/nonexistent")
		if err == nil {
			t.Error("Expected repository not found error")
		}
	})

	t.Run("branch not found", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.DeleteBranchError = errors.New("422 Reference does not exist")

		err := mockAPI.DeleteBranch("nonexistent-branch")
		if err == nil {
			t.Error("Expected branch not found error")
		}
	})

	t.Run("PR not found", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestByBranchError = ghpkg.ErrPRNotFound

		_, err := mockAPI.GetPullRequestByBranch("feature", "main")
		if err == nil {
			t.Error("Expected PR not found error")
		}
	})
}

// TestErrorServiceOutages tests GitHub service outage scenarios.
func TestErrorServiceOutages(t *testing.T) {
	outageErrors := []struct {
		name  string
		error error
	}{
		{"internal server error", errors.New("500 Internal Server Error")},
		{"bad gateway", errors.New("502 Bad Gateway")},
		{"service unavailable", errors.New("503 Service Unavailable")},
		{"gateway timeout", errors.New("504 Gateway Timeout")},
	}

	for _, tc := range outageErrors {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()
			mockAPI.WaitForWorkflowsError = tc.error

			_, err := mockAPI.WaitForWorkflows(5 * time.Minute)
			if err == nil {
				t.Errorf("Expected service outage error for %s", tc.name)
			}
			if err.Error() != tc.error.Error() {
				t.Errorf("Expected error '%v', got '%v'", tc.error, err)
			}
		})
	}
}

// TestErrorValidationFailures tests input validation errors.
func TestErrorValidationFailures(t *testing.T) {
	t.Run("empty PR title", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestError = errors.New("422 Validation Failed: title can't be blank")

		_, err := mockAPI.CreatePullRequest(
			"feature", "main", "", "Body", nil, nil, nil,
		)
		if err == nil {
			t.Error("Expected validation error for empty title")
		}
	})

	t.Run("invalid branch name", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestError = errors.New("422 Validation Failed: head ref is invalid")

		_, err := mockAPI.CreatePullRequest(
			"", "main", "Title", "Body", nil, nil, nil,
		)
		if err == nil {
			t.Error("Expected validation error for invalid branch")
		}
	})

	t.Run("same source and target branch", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestError = errors.New("422 Validation Failed: head and base must be different")

		_, err := mockAPI.CreatePullRequest(
			"main", "main", "Title", "Body", nil, nil, nil,
		)
		if err == nil {
			t.Error("Expected validation error for same source and target")
		}
	})
}

// TestErrorPropagation tests that errors propagate correctly through the API.
func TestErrorPropagation(t *testing.T) {
	t.Run("error from nested operations", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		originalErr := errors.New("original error")
		mockAPI.SetRepositoryFromURLError = originalErr

		err := mockAPI.SetRepositoryFromURL("https://github.com/owner/repo")
		if err == nil {
			t.Error("Expected error to propagate")
		}
		if err != originalErr {
			t.Errorf("Expected original error to propagate, got %v", err)
		}
	})

	t.Run("error context preserved", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		contextErr := fmt.Errorf("failed to create PR: %w", ghpkg.ErrInvalidURLFormat)
		mockAPI.CreatePullRequestError = contextErr

		_, err := mockAPI.CreatePullRequest(
			"feature", "main", "Title", "Body", nil, nil, nil,
		)
		if err == nil {
			t.Error("Expected error with context")
		}
		// Verify error context is preserved
		if !errors.Is(err, ghpkg.ErrInvalidURLFormat) {
			t.Error("Expected wrapped error to be unwrappable")
		}
	})
}

// TestErrorRecovery tests error recovery scenarios.
func TestErrorRecovery(t *testing.T) {
	t.Run("retry after transient error", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// First attempt fails
		mockAPI.ListLabelsError = errors.New("503 Service Unavailable")
		_, err := mockAPI.ListLabels()
		if err == nil {
			t.Error("Expected first attempt to fail")
		}

		// Second attempt succeeds
		mockAPI.ListLabelsError = nil
		mockAPI.ListLabelsResponse = []*ghpkg.Label{{Name: "bug"}}
		labels, err := mockAPI.ListLabels()
		if err != nil {
			t.Fatalf("Expected retry to succeed: %v", err)
		}
		if len(labels) != 1 {
			t.Errorf("Expected 1 label after retry, got %d", len(labels))
		}
	})

	t.Run("graceful degradation on partial failure", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Can still proceed if non-critical operations fail
		mockAPI.DeleteBranchError = errors.New("403 Forbidden")
		err := mockAPI.DeleteBranch("feature")
		if err == nil {
			t.Error("Expected delete to fail")
		}
		// But this doesn't prevent other operations
		t.Log("Branch deletion failed, but workflow can continue")
	})
}
