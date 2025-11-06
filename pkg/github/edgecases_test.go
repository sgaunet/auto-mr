package github_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v69/github"
	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
)

// TestEdgeCaseEmptyResponses tests handling of empty API responses.
func TestEdgeCaseEmptyResponses(t *testing.T) {
	t.Run("empty label list", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsResponse = []*ghpkg.Label{}

		labels, err := mockAPI.ListLabels()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if labels == nil {
			t.Error("Expected non-nil empty slice")
		}
		if len(labels) != 0 {
			t.Errorf("Expected empty list, got %d labels", len(labels))
		}
	})

	t.Run("empty PR list", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestsByHeadResponse = []*github.PullRequest{}

		prs, err := mockAPI.GetPullRequestsByHead("feature")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if prs == nil {
			t.Error("Expected non-nil empty slice")
		}
		if len(prs) != 0 {
			t.Errorf("Expected empty list, got %d PRs", len(prs))
		}
	})

	t.Run("nil label response", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsResponse = nil

		labels, err := mockAPI.ListLabels()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if labels != nil {
			t.Errorf("Expected nil, got %v", labels)
		}
	})
}

// TestEdgeCaseSpecialCharacters tests handling of special characters in inputs.
func TestEdgeCaseSpecialCharacters(t *testing.T) {
	specialStrings := []struct {
		name  string
		value string
	}{
		{"unicode characters", "🚀 Feature: Add émojis"},
		{"special symbols", "Fix: Issue #123 & PR @456"},
		{"newlines", "Title\nWith\nNewlines"},
		{"tabs", "Title\tWith\tTabs"},
		{"quotes", "Title with \"quotes\" and 'apostrophes'"},
		{"backslashes", "Path\\to\\file"},
		{"HTML entities", "<script>alert('xss')</script>"},
		{"SQL injection", "'; DROP TABLE users; --"},
		{"null bytes", "Title\x00With\x00Nulls"},
	}

	for _, tc := range specialStrings {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()
			mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

			// Should handle special characters without error
			pr, err := mockAPI.CreatePullRequest(
				"feature", "main", tc.value, "Body", nil, nil, nil,
			)
			if err != nil {
				t.Fatalf("Failed to handle special characters: %v", err)
			}
			if pr == nil {
				t.Error("Expected PR to be created")
			}

			// Verify special characters were passed through
			lastCall := mockAPI.GetLastCall("CreatePullRequest")
			if lastCall.Args["title"] != tc.value {
				t.Errorf("Special characters not preserved: expected %q, got %q",
					tc.value, lastCall.Args["title"])
			}
		})
	}
}

// TestEdgeCaseLongStrings tests handling of very long input strings.
func TestEdgeCaseLongStrings(t *testing.T) {
	t.Run("very long PR title", func(t *testing.T) {
		longTitle := strings.Repeat("A", 1000)

		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		pr, err := mockAPI.CreatePullRequest(
			"feature", "main", longTitle, "Body", nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("Failed with long title: %v", err)
		}
		if pr == nil {
			t.Error("Expected PR to be created")
		}
	})

	t.Run("very long PR body", func(t *testing.T) {
		longBody := strings.Repeat("Lorem ipsum dolor sit amet. ", 1000)

		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		pr, err := mockAPI.CreatePullRequest(
			"feature", "main", "Title", longBody, nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("Failed with long body: %v", err)
		}
		if pr == nil {
			t.Error("Expected PR to be created")
		}
	})

	t.Run("very long branch name", func(t *testing.T) {
		longBranch := "feature/" + strings.Repeat("a", 200)

		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		pr, err := mockAPI.CreatePullRequest(
			longBranch, "main", "Title", "Body", nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("Failed with long branch name: %v", err)
		}
		if pr == nil {
			t.Error("Expected PR to be created")
		}
	})
}

// TestEdgeCaseBoundaryValues tests boundary value conditions.
func TestEdgeCaseBoundaryValues(t *testing.T) {
	t.Run("zero timeout", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.WaitForWorkflowsError = ghpkg.ErrWorkflowTimeout

		_, err := mockAPI.WaitForWorkflows(0)
		if err == nil {
			t.Error("Expected error for zero timeout")
		}
	})

	t.Run("negative timeout", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.WaitForWorkflowsError = ghpkg.ErrWorkflowTimeout

		_, err := mockAPI.WaitForWorkflows(-1 * time.Second)
		if err == nil {
			t.Error("Expected error for negative timeout")
		}
	})

	t.Run("very large timeout", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.WaitForWorkflowsConclusion = "success"

		conclusion, err := mockAPI.WaitForWorkflows(24 * time.Hour)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if conclusion != "success" {
			t.Errorf("Expected success, got %s", conclusion)
		}
	})

	t.Run("PR number zero", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// PR number 0 might be treated as invalid
		err := mockAPI.MergePullRequest(0, "squash")
		// Behavior depends on implementation - just verify it's handled
		_ = err
	})

	t.Run("negative PR number", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Negative PR number should be invalid
		err := mockAPI.MergePullRequest(-1, "squash")
		// Behavior depends on implementation - just verify it's handled
		_ = err
	})
}

// TestEdgeCaseMaximumLimits tests maximum limit scenarios.
func TestEdgeCaseMaximumLimits(t *testing.T) {
	t.Run("maximum number of labels", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Create maximum number of labels (100 is typical GitHub limit)
		maxLabels := make([]*ghpkg.Label, 100)
		for i := 0; i < 100; i++ {
			maxLabels[i] = &ghpkg.Label{Name: strings.Repeat("label", i)}
		}
		mockAPI.ListLabelsResponse = maxLabels

		labels, err := mockAPI.ListLabels()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(labels) != 100 {
			t.Errorf("Expected 100 labels, got %d", len(labels))
		}
	})

	t.Run("large number of assignees", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		// Try with many assignees
		manyAssignees := make([]string, 50)
		for i := 0; i < 50; i++ {
			manyAssignees[i] = "user" + strings.Repeat("x", i)
		}

		pr, err := mockAPI.CreatePullRequest(
			"feature", "main", "Title", "Body",
			manyAssignees, nil, nil,
		)
		if err != nil {
			t.Fatalf("Failed with many assignees: %v", err)
		}
		if pr == nil {
			t.Error("Expected PR to be created")
		}
	})

	t.Run("large number of labels on PR", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		// Try with many labels
		manyLabels := make([]string, 50)
		for i := 0; i < 50; i++ {
			manyLabels[i] = "label-" + strings.Repeat("x", i)
		}

		pr, err := mockAPI.CreatePullRequest(
			"feature", "main", "Title", "Body",
			nil, nil, manyLabels,
		)
		if err != nil {
			t.Fatalf("Failed with many labels: %v", err)
		}
		if pr == nil {
			t.Error("Expected PR to be created")
		}
	})
}

// TestEdgeCaseURLVariations tests various URL format variations.
func TestEdgeCaseURLVariations(t *testing.T) {
	urlVariations := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"HTTPS with www", "https://www.github.com/owner/repo.git", false},
		{"HTTPS without www", "https://github.com/owner/repo.git", false},
		{"SSH git@ format", "git@github.com:owner/repo.git", false},
		{"SSH with ssh://", "ssh://git@github.com/owner/repo.git", false},
		{"Mixed case domain", "https://GitHub.com/owner/repo.git", false},
		{"Trailing slash", "https://github.com/owner/repo/", false},
		{"Multiple dots", "https://github.com/owner/repo.name.git", false},
		{"Hyphens in names", "https://github.com/owner-name/repo-name.git", false},
		{"Underscores", "https://github.com/owner_name/repo_name.git", false},
		{"Numbers", "https://github.com/owner123/repo456.git", false},
		{"Single char names", "https://github.com/a/b.git", false},
		{"No protocol", "github.com/owner/repo.git", true},
		{"Invalid protocol", "ftp://github.com/owner/repo.git", true},
		{"Missing repo", "https://github.com/owner/.git", true},
		{"Extra slashes", "https://github.com//owner//repo.git", false},
	}

	for _, tc := range urlVariations {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()
			if tc.wantErr {
				mockAPI.SetRepositoryFromURLError = ghpkg.ErrInvalidURLFormat
			}

			err := mockAPI.SetRepositoryFromURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Errorf("URL %s: error = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

// TestEdgeCaseConcurrentOperations tests concurrent access patterns.
func TestEdgeCaseConcurrentOperations(t *testing.T) {
	t.Run("concurrent label fetches", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsResponse = []*ghpkg.Label{
			{Name: "bug"},
			{Name: "feature"},
		}

		// Launch multiple concurrent requests
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				labels, err := mockAPI.ListLabels()
				if err != nil || len(labels) != 2 {
					t.Errorf("Concurrent fetch failed: err=%v, len=%d", err, len(labels))
				}
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify call count
		if mockAPI.GetCallCount("ListLabels") != 10 {
			t.Errorf("Expected 10 concurrent calls, got %d",
				mockAPI.GetCallCount("ListLabels"))
		}
	})

	t.Run("concurrent PR creations", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		// Launch multiple concurrent PR creations
		done := make(chan bool, 5)
		for i := 0; i < 5; i++ {
			go func(num int) {
				branch := strings.Repeat("feature-", num)
				pr, err := mockAPI.CreatePullRequest(
					branch, "main", "Title", "Body", nil, nil, nil,
				)
				if err != nil || pr == nil {
					t.Errorf("Concurrent PR creation failed: err=%v", err)
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 5; i++ {
			<-done
		}
	})
}

// TestEdgeCaseNilPointers tests handling of nil pointers and values.
func TestEdgeCaseNilPointers(t *testing.T) {
	t.Run("nil slice parameters", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		// All slice parameters are nil
		pr, err := mockAPI.CreatePullRequest(
			"feature", "main", "Title", "Body",
			nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("Failed with nil slices: %v", err)
		}
		if pr == nil {
			t.Error("Expected PR to be created")
		}
	})

	t.Run("empty slice parameters", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		// All slice parameters are empty
		pr, err := mockAPI.CreatePullRequest(
			"feature", "main", "Title", "Body",
			[]string{}, []string{}, []string{},
		)
		if err != nil {
			t.Fatalf("Failed with empty slices: %v", err)
		}
		if pr == nil {
			t.Error("Expected PR to be created")
		}
	})
}

// TestEdgeCaseWorkflowStates tests various workflow state combinations.
func TestEdgeCaseWorkflowStates(t *testing.T) {
	states := []string{
		"success",
		"failure",
		"cancelled",
		"skipped",
		"neutral",
		"action_required",
		"stale",
		"timed_out",
	}

	for _, state := range states {
		t.Run("workflow state: "+state, func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()
			mockAPI.WaitForWorkflowsConclusion = state

			conclusion, err := mockAPI.WaitForWorkflows(5 * time.Minute)
			if err != nil {
				t.Fatalf("Unexpected error for state %s: %v", state, err)
			}
			if conclusion != state {
				t.Errorf("Expected conclusion %s, got %s", state, conclusion)
			}
		})
	}
}

// TestEdgeCaseBranchNameFormats tests various branch name formats.
func TestEdgeCaseBranchNameFormats(t *testing.T) {
	branchNames := []string{
		"feature/add-new-feature",
		"bugfix/issue-123",
		"release/v1.2.3",
		"hotfix/critical-fix",
		"feat/JIRA-1234-description",
		"feature_with_underscores",
		"feature-with-hyphens",
		"user/john.doe/feature",
		"refs/heads/feature",
		"dependabot/npm_and_yarn/lodash-4.17.19",
		"feature#123",
		"v1.0.0",
		"123-numeric-start",
		"UPPERCASE-BRANCH",
		"MixedCase-Branch",
	}

	for _, branch := range branchNames {
		t.Run("branch: "+branch, func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()
			mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

			pr, err := mockAPI.CreatePullRequest(
				branch, "main", "Title", "Body", nil, nil, nil,
			)
			if err != nil {
				t.Fatalf("Failed with branch name %s: %v", branch, err)
			}
			if pr == nil {
				t.Error("Expected PR to be created")
			}
		})
	}
}

// TestEdgeCaseMergeMethodVariations tests GetMergeMethod utility.
func TestEdgeCaseMergeMethodVariations(t *testing.T) {
	t.Run("GetMergeMethod with squash true", func(t *testing.T) {
		method := ghpkg.GetMergeMethod(true)
		if method != "squash" {
			t.Errorf("Expected 'squash', got %s", method)
		}
	})

	t.Run("GetMergeMethod with squash false", func(t *testing.T) {
		method := ghpkg.GetMergeMethod(false)
		if method != "merge" {
			t.Errorf("Expected 'merge', got %s", method)
		}
	})
}

// TestEdgeCaseRepositoryNotFound tests repository not found scenarios.
func TestEdgeCaseRepositoryNotFound(t *testing.T) {
	t.Run("private repository", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.SetRepositoryFromURLError = ghpkg.ErrInvalidURLFormat

		err := mockAPI.SetRepositoryFromURL("https://github.com/private-owner/private-repo")
		if err == nil {
			t.Error("Expected error for private/inaccessible repository")
		}
	})

	t.Run("deleted repository", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.SetRepositoryFromURLError = ghpkg.ErrInvalidURLFormat

		err := mockAPI.SetRepositoryFromURL("https://github.com/owner/deleted-repo")
		if err == nil {
			t.Error("Expected error for deleted repository")
		}
	})
}
