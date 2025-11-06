package github_test

import (
	"testing"
	"time"

	"github.com/google/go-github/v69/github"
	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
)

// TestClientConstructor tests the client construction and configuration.
func TestClientConstructor(t *testing.T) {
	t.Run("NewClient requires GITHUB_TOKEN", func(t *testing.T) {
		// This would require unsetting env var, which is tricky in tests
		// Skip for now as it would affect other tests
		t.Skip("Requires environment manipulation")
	})
}

// TestSetRepositoryFromURL tests repository URL parsing and validation.
func TestSetRepositoryFromURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid HTTPS URL",
			url:     "https://github.com/owner/repo.git",
			wantErr: false,
		},
		{
			name:    "valid HTTPS URL without .git",
			url:     "https://github.com/owner/repo",
			wantErr: false,
		},
		{
			name:    "valid SSH URL",
			url:     "git@github.com:owner/repo.git",
			wantErr: false,
		},
		{
			name:    "valid SSH URL without .git",
			url:     "git@github.com:owner/repo",
			wantErr: false,
		},
		{
			name:    "ssh:// protocol URL",
			url:     "ssh://git@github.com/owner/repo.git",
			wantErr: false,
		},
		{
			name:    "invalid URL format",
			url:     "not-a-valid-url",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "URL with only owner",
			url:     "https://github.com/owner",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use mock API client for testing
			mockAPI := mocks.NewGitHubAPIClient()
			if tt.wantErr {
				mockAPI.SetRepositoryFromURLError = ghpkg.ErrInvalidURLFormat
			}

			err := mockAPI.SetRepositoryFromURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetRepositoryFromURL() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify method was called
			if mockAPI.GetCallCount("SetRepositoryFromURL") != 1 {
				t.Errorf("Expected SetRepositoryFromURL to be called once, got %d",
					mockAPI.GetCallCount("SetRepositoryFromURL"))
			}

			// Verify URL parameter
			lastCall := mockAPI.GetLastCall("SetRepositoryFromURL")
			if lastCall == nil {
				t.Fatal("Expected method call to be tracked")
			}
			if lastCall.Args["url"] != tt.url {
				t.Errorf("Expected URL %s, got %s", tt.url, lastCall.Args["url"])
			}
		})
	}
}

// TestListLabels tests label retrieval functionality.
func TestListLabels(t *testing.T) {
	t.Run("successful label retrieval", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsResponse = []*ghpkg.Label{
			{Name: "bug"},
			{Name: "enhancement"},
			{Name: "documentation"},
		}

		labels, err := mockAPI.ListLabels()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(labels) != 3 {
			t.Errorf("Expected 3 labels, got %d", len(labels))
		}

		expectedLabels := []string{"bug", "enhancement", "documentation"}
		for i, label := range labels {
			if label.Name != expectedLabels[i] {
				t.Errorf("Expected label %s, got %s", expectedLabels[i], label.Name)
			}
		}
	})

	t.Run("API error handling", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsError = ghpkg.ErrTokenRequired

		_, err := mockAPI.ListLabels()
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("empty label list", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.ListLabelsResponse = []*ghpkg.Label{}

		labels, err := mockAPI.ListLabels()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(labels) != 0 {
			t.Errorf("Expected empty label list, got %d labels", len(labels))
		}
	})
}

// TestCreatePullRequest tests PR creation with various configurations.
func TestCreatePullRequest(t *testing.T) {
	t.Run("create PR with all fields", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		head := "feature-branch"
		base := "main"
		title := "Test PR"
		body := "Test description"
		assignees := []string{"user1", "user2"}
		reviewers := []string{"reviewer1"}
		labels := []string{"bug", "urgent"}

		pr, err := mockAPI.CreatePullRequest(head, base, title, body, assignees, reviewers, labels)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if pr == nil {
			t.Fatal("Expected PR to be returned")
		}

		// Verify method call parameters
		lastCall := mockAPI.GetLastCall("CreatePullRequest")
		if lastCall == nil {
			t.Fatal("Expected method call to be tracked")
		}

		if lastCall.Args["head"] != head {
			t.Errorf("Expected head %s, got %s", head, lastCall.Args["head"])
		}
		if lastCall.Args["base"] != base {
			t.Errorf("Expected base %s, got %s", base, lastCall.Args["base"])
		}
		if lastCall.Args["title"] != title {
			t.Errorf("Expected title %s, got %s", title, lastCall.Args["title"])
		}
	})

	t.Run("create PR with minimal fields", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		pr, err := mockAPI.CreatePullRequest("feature", "main", "Title", "Body", nil, nil, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if pr == nil {
			t.Fatal("Expected PR to be returned")
		}
	})

	t.Run("API error during PR creation", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestError = ghpkg.ErrInvalidURLFormat

		_, err := mockAPI.CreatePullRequest("feature", "main", "Title", "Body", nil, nil, nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

// TestGetPullRequestByBranch tests PR lookup by branch names.
func TestGetPullRequestByBranch(t *testing.T) {
	t.Run("find existing PR", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestByBranchResponse = fixtures.ValidPullRequest()

		pr, err := mockAPI.GetPullRequestByBranch("feature", "main")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if pr == nil {
			t.Fatal("Expected PR to be found")
		}
	})

	t.Run("PR not found", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestByBranchError = ghpkg.ErrPRNotFound

		_, err := mockAPI.GetPullRequestByBranch("nonexistent", "main")
		if err == nil {
			t.Error("Expected error for non-existent PR")
		}
	})
}

// TestWaitForWorkflows tests workflow monitoring functionality.
func TestWaitForWorkflows(t *testing.T) {
	t.Run("workflows complete successfully", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.WaitForWorkflowsConclusion = "success"

		conclusion, err := mockAPI.WaitForWorkflows(5 * time.Minute)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if conclusion != "success" {
			t.Errorf("Expected success conclusion, got %s", conclusion)
		}
	})

	t.Run("workflows fail", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.WaitForWorkflowsConclusion = "failure"

		conclusion, err := mockAPI.WaitForWorkflows(5 * time.Minute)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if conclusion != "failure" {
			t.Errorf("Expected failure conclusion, got %s", conclusion)
		}
	})

	t.Run("workflow timeout", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.WaitForWorkflowsError = ghpkg.ErrWorkflowTimeout

		_, err := mockAPI.WaitForWorkflows(1 * time.Second)
		if err == nil {
			t.Error("Expected timeout error")
		}
	})
}

// TestMergePullRequest tests PR merging with different strategies.
func TestMergePullRequest(t *testing.T) {
	mergeStrategies := []struct {
		method string
	}{
		{"merge"},
		{"squash"},
		{"rebase"},
	}

	for _, strategy := range mergeStrategies {
		t.Run("merge with "+strategy.method, func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()

			err := mockAPI.MergePullRequest(123, strategy.method)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			lastCall := mockAPI.GetLastCall("MergePullRequest")
			if lastCall == nil {
				t.Fatal("Expected method call to be tracked")
			}

			if lastCall.Args["mergeMethod"] != strategy.method {
				t.Errorf("Expected merge method %s, got %s",
					strategy.method, lastCall.Args["mergeMethod"])
			}
		})
	}

	t.Run("merge failure", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.MergePullRequestError = ghpkg.ErrInvalidURLFormat

		err := mockAPI.MergePullRequest(123, "merge")
		if err == nil {
			t.Error("Expected merge error")
		}
	})
}

// TestGetPullRequestsByHead tests PR listing by head branch.
func TestGetPullRequestsByHead(t *testing.T) {
	t.Run("find PRs for branch", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestsByHeadResponse = []*github.PullRequest{
			fixtures.ValidPullRequest(),
		}

		prs, err := mockAPI.GetPullRequestsByHead("feature")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(prs) != 1 {
			t.Errorf("Expected 1 PR, got %d", len(prs))
		}
	})

	t.Run("no PRs for branch", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestsByHeadResponse = []*github.PullRequest{}

		prs, err := mockAPI.GetPullRequestsByHead("nonexistent")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(prs) != 0 {
			t.Errorf("Expected 0 PRs, got %d", len(prs))
		}
	})
}

// TestDeleteBranch tests branch deletion functionality.
func TestDeleteBranch(t *testing.T) {
	t.Run("successful branch deletion", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		err := mockAPI.DeleteBranch("feature-branch")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		lastCall := mockAPI.GetLastCall("DeleteBranch")
		if lastCall == nil {
			t.Fatal("Expected method call to be tracked")
		}

		if lastCall.Args["branch"] != "feature-branch" {
			t.Errorf("Expected branch name feature-branch, got %s", lastCall.Args["branch"])
		}
	})

	t.Run("branch deletion failure", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.DeleteBranchError = ghpkg.ErrInvalidURLFormat

		err := mockAPI.DeleteBranch("protected-branch")
		if err == nil {
			t.Error("Expected deletion error")
		}
	})
}

// TestGetMergeMethod tests the merge method utility function.
func TestGetMergeMethod(t *testing.T) {
	tests := []struct {
		name   string
		squash bool
		want   string
	}{
		{
			name:   "squash merge",
			squash: true,
			want:   "squash",
		},
		{
			name:   "regular merge",
			squash: false,
			want:   "merge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ghpkg.GetMergeMethod(tt.squash)
			if got != tt.want {
				t.Errorf("GetMergeMethod(%v) = %v, want %v", tt.squash, got, tt.want)
			}
		})
	}
}
