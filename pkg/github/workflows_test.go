package github_test

import (
	"testing"
	"time"

	"github.com/google/go-github/v69/github"
	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
)

// TestWorkflowPRCreationToMerge tests the complete PR lifecycle from creation to merge.
func TestWorkflowPRCreationToMerge(t *testing.T) {
	t.Run("successful PR workflow", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Step 1: Create PR
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()
		pr, err := mockAPI.CreatePullRequest(
			"feature", "main", "Test PR", "Description",
			[]string{"user1"}, []string{"reviewer1"}, []string{"bug"},
		)
		if err != nil {
			t.Fatalf("Failed to create PR: %v", err)
		}
		if pr == nil {
			t.Fatal("Expected PR to be created")
		}

		// Step 2: Wait for workflows
		mockAPI.WaitForWorkflowsConclusion = "success"
		conclusion, err := mockAPI.WaitForWorkflows(5 * time.Minute)
		if err != nil {
			t.Fatalf("Workflow wait failed: %v", err)
		}
		if conclusion != "success" {
			t.Errorf("Expected success conclusion, got %s", conclusion)
		}

		// Step 3: Merge PR
		err = mockAPI.MergePullRequest(*pr.Number, "squash")
		if err != nil {
			t.Fatalf("Failed to merge PR: %v", err)
		}

		// Verify complete workflow
		if mockAPI.GetCallCount("CreatePullRequest") != 1 {
			t.Error("Expected CreatePullRequest to be called once")
		}
		if mockAPI.GetCallCount("WaitForWorkflows") != 1 {
			t.Error("Expected WaitForWorkflows to be called once")
		}
		if mockAPI.GetCallCount("MergePullRequest") != 1 {
			t.Error("Expected MergePullRequest to be called once")
		}
	})

	t.Run("PR workflow with failing checks", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Create PR
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()
		_, err := mockAPI.CreatePullRequest(
			"feature", "main", "Test PR", "Description",
			nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("Failed to create PR: %v", err)
		}

		// Wait for workflows - they fail
		mockAPI.WaitForWorkflowsConclusion = "failure"
		conclusion, err := mockAPI.WaitForWorkflows(5 * time.Minute)
		if err != nil {
			t.Fatalf("Workflow wait failed: %v", err)
		}
		if conclusion != "failure" {
			t.Errorf("Expected failure conclusion, got %s", conclusion)
		}

		// Should not merge failed PR
		// In a real workflow, the caller would check conclusion before merging
		if conclusion != "success" {
			// Don't attempt merge
			t.Log("Correctly avoiding merge of failed PR")
		}

		// Verify PR was created but not merged
		if mockAPI.GetCallCount("CreatePullRequest") != 1 {
			t.Error("Expected CreatePullRequest to be called once")
		}
		if mockAPI.GetCallCount("MergePullRequest") != 0 {
			t.Error("Expected MergePullRequest not to be called for failed checks")
		}
	})
}

// TestWorkflowPRUpdateAndRetry tests PR updates and retry scenarios.
func TestWorkflowPRUpdateAndRetry(t *testing.T) {
	t.Run("retry after fixing issues", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Create PR
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()
		pr, _ := mockAPI.CreatePullRequest(
			"feature", "main", "Test PR", "Description",
			nil, nil, nil,
		)

		// First attempt - workflows fail
		mockAPI.WaitForWorkflowsConclusion = "failure"
		conclusion1, _ := mockAPI.WaitForWorkflows(5 * time.Minute)
		if conclusion1 != "failure" {
			t.Errorf("Expected first attempt to fail, got %s", conclusion1)
		}

		// After fixing code, retry - workflows succeed
		mockAPI.WaitForWorkflowsConclusion = "success"
		conclusion2, err := mockAPI.WaitForWorkflows(5 * time.Minute)
		if err != nil {
			t.Fatalf("Second workflow wait failed: %v", err)
		}
		if conclusion2 != "success" {
			t.Errorf("Expected second attempt to succeed, got %s", conclusion2)
		}

		// Now merge
		err = mockAPI.MergePullRequest(*pr.Number, "squash")
		if err != nil {
			t.Fatalf("Failed to merge PR: %v", err)
		}

		// Verify retry pattern
		if mockAPI.GetCallCount("WaitForWorkflows") != 2 {
			t.Errorf("Expected WaitForWorkflows to be called twice, got %d",
				mockAPI.GetCallCount("WaitForWorkflows"))
		}
	})
}

// TestWorkflowConcurrentPRs tests handling multiple PRs concurrently.
func TestWorkflowConcurrentPRs(t *testing.T) {
	t.Run("create multiple PRs", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()

		// Create multiple PRs
		branches := []string{"feature-1", "feature-2", "feature-3"}
		for _, branch := range branches {
			pr, err := mockAPI.CreatePullRequest(
				branch, "main", "Test PR", "Description",
				nil, nil, nil,
			)
			if err != nil {
				t.Fatalf("Failed to create PR for %s: %v", branch, err)
			}
			if pr == nil {
				t.Fatalf("Expected PR to be created for %s", branch)
			}
		}

		// Verify all PRs were created
		if mockAPI.GetCallCount("CreatePullRequest") != 3 {
			t.Errorf("Expected 3 CreatePullRequest calls, got %d",
				mockAPI.GetCallCount("CreatePullRequest"))
		}
	})

	t.Run("list PRs for branch", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()
		mockAPI.GetPullRequestsByHeadResponse = []*github.PullRequest{
			fixtures.ValidPullRequest(),
		}

		prs, err := mockAPI.GetPullRequestsByHead("feature-branch")
		if err != nil {
			t.Fatalf("Failed to list PRs: %v", err)
		}

		if len(prs) != 1 {
			t.Errorf("Expected 1 PR, got %d", len(prs))
		}
	})
}

// TestWorkflowBranchCleanup tests branch deletion after merge.
func TestWorkflowBranchCleanup(t *testing.T) {
	t.Run("delete branch after successful merge", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Create and merge PR
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()
		pr, _ := mockAPI.CreatePullRequest(
			"feature", "main", "Test PR", "Description",
			nil, nil, nil,
		)

		mockAPI.WaitForWorkflowsConclusion = "success"
		_, _ = mockAPI.WaitForWorkflows(5 * time.Minute)

		err := mockAPI.MergePullRequest(*pr.Number, "squash")
		if err != nil {
			t.Fatalf("Failed to merge PR: %v", err)
		}

		// Clean up branch
		err = mockAPI.DeleteBranch("feature")
		if err != nil {
			t.Fatalf("Failed to delete branch: %v", err)
		}

		// Verify cleanup
		if mockAPI.GetCallCount("DeleteBranch") != 1 {
			t.Error("Expected DeleteBranch to be called once")
		}

		lastCall := mockAPI.GetLastCall("DeleteBranch")
		if lastCall.Args["branch"] != "feature" {
			t.Errorf("Expected to delete 'feature' branch, got %v", lastCall.Args["branch"])
		}
	})
}

// TestWorkflowFindExistingPR tests finding and working with existing PRs.
func TestWorkflowFindExistingPR(t *testing.T) {
	t.Run("find and merge existing PR", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Find existing PR
		mockAPI.GetPullRequestByBranchResponse = fixtures.ValidPullRequest()
		pr, err := mockAPI.GetPullRequestByBranch("feature", "main")
		if err != nil {
			t.Fatalf("Failed to find PR: %v", err)
		}
		if pr == nil {
			t.Fatal("Expected to find existing PR")
		}

		// Wait for workflows
		mockAPI.WaitForWorkflowsConclusion = "success"
		conclusion, _ := mockAPI.WaitForWorkflows(5 * time.Minute)
		if conclusion != "success" {
			t.Errorf("Expected success, got %s", conclusion)
		}

		// Merge existing PR
		err = mockAPI.MergePullRequest(*pr.Number, "merge")
		if err != nil {
			t.Fatalf("Failed to merge existing PR: %v", err)
		}

		// Verify workflow
		if mockAPI.GetCallCount("GetPullRequestByBranch") != 1 {
			t.Error("Expected GetPullRequestByBranch to be called once")
		}
		if mockAPI.GetCallCount("CreatePullRequest") != 0 {
			t.Error("Should not create new PR when one exists")
		}
	})

	t.Run("handle no existing PR", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Try to find non-existent PR
		mockAPI.GetPullRequestByBranchError = ghpkg.ErrPRNotFound
		_, err := mockAPI.GetPullRequestByBranch("nonexistent", "main")
		if err == nil {
			t.Error("Expected error for non-existent PR")
		}

		// Should create new PR
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()
		mockAPI.CreatePullRequestError = nil
		pr, err := mockAPI.CreatePullRequest(
			"nonexistent", "main", "New PR", "Description",
			nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("Failed to create new PR: %v", err)
		}
		if pr == nil {
			t.Fatal("Expected new PR to be created")
		}
	})
}

// TestWorkflowMergeStrategies tests different merge strategies in workflows.
func TestWorkflowMergeStrategies(t *testing.T) {
	strategies := []struct {
		name   string
		method string
	}{
		{"merge commit", "merge"},
		{"squash merge", "squash"},
		{"rebase merge", "rebase"},
	}

	for _, strategy := range strategies {
		t.Run(strategy.name, func(t *testing.T) {
			mockAPI := mocks.NewGitHubAPIClient()

			// Create PR
			mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()
			pr, _ := mockAPI.CreatePullRequest(
				"feature", "main", "Test PR", "Description",
				nil, nil, nil,
			)

			// Wait for success
			mockAPI.WaitForWorkflowsConclusion = "success"
			_, _ = mockAPI.WaitForWorkflows(5 * time.Minute)

			// Merge with specific strategy
			err := mockAPI.MergePullRequest(*pr.Number, strategy.method)
			if err != nil {
				t.Fatalf("Failed to merge with %s: %v", strategy.method, err)
			}

			// Verify merge method
			lastCall := mockAPI.GetLastCall("MergePullRequest")
			if lastCall.Args["mergeMethod"] != strategy.method {
				t.Errorf("Expected merge method %s, got %v",
					strategy.method, lastCall.Args["mergeMethod"])
			}
		})
	}
}

// TestWorkflowWithLabels tests PR workflow with label management.
func TestWorkflowWithLabels(t *testing.T) {
	t.Run("create PR with labels and merge", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// List available labels
		mockAPI.ListLabelsResponse = []*ghpkg.Label{
			{Name: "bug"},
			{Name: "enhancement"},
			{Name: "urgent"},
		}
		labels, _ := mockAPI.ListLabels()
		if len(labels) != 3 {
			t.Errorf("Expected 3 labels, got %d", len(labels))
		}

		// Create PR with selected labels
		mockAPI.CreatePullRequestResponse = fixtures.ValidPullRequest()
		pr, err := mockAPI.CreatePullRequest(
			"bugfix", "main", "Fix critical bug", "Description",
			nil, nil, []string{"bug", "urgent"},
		)
		if err != nil {
			t.Fatalf("Failed to create PR with labels: %v", err)
		}

		// Verify labels were passed
		lastCall := mockAPI.GetLastCall("CreatePullRequest")
		passedLabels := lastCall.Args["labels"].([]string)
		if len(passedLabels) != 2 {
			t.Errorf("Expected 2 labels, got %d", len(passedLabels))
		}
		if passedLabels[0] != "bug" || passedLabels[1] != "urgent" {
			t.Errorf("Labels not passed correctly: %v", passedLabels)
		}

		// Complete workflow
		mockAPI.WaitForWorkflowsConclusion = "success"
		_, _ = mockAPI.WaitForWorkflows(5 * time.Minute)
		_ = mockAPI.MergePullRequest(*pr.Number, "squash")
	})
}

// TestWorkflowTimeouts tests timeout handling in workflows.
func TestWorkflowTimeouts(t *testing.T) {
	t.Run("short timeout triggers error", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Simulate timeout
		mockAPI.WaitForWorkflowsError = ghpkg.ErrWorkflowTimeout

		_, err := mockAPI.WaitForWorkflows(1 * time.Millisecond)
		if err == nil {
			t.Error("Expected timeout error")
		}
		if err != ghpkg.ErrWorkflowTimeout {
			t.Errorf("Expected ErrWorkflowTimeout, got %v", err)
		}
	})

	t.Run("long timeout allows completion", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		mockAPI.WaitForWorkflowsConclusion = "success"
		conclusion, err := mockAPI.WaitForWorkflows(30 * time.Minute)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if conclusion != "success" {
			t.Errorf("Expected success, got %s", conclusion)
		}
	})
}

// TestWorkflowStateValidation tests PR state validation throughout workflow.
func TestWorkflowStateValidation(t *testing.T) {
	t.Run("verify PR exists before operations", func(t *testing.T) {
		mockAPI := mocks.NewGitHubAPIClient()

		// Try to find PR first
		mockAPI.GetPullRequestByBranchResponse = fixtures.ValidPullRequest()
		pr, err := mockAPI.GetPullRequestByBranch("feature", "main")
		if err != nil {
			t.Fatalf("Failed to find PR: %v", err)
		}

		// Verify PR state before proceeding
		if pr.Number == nil {
			t.Fatal("PR should have a number")
		}
		if pr.Head == nil || pr.Head.SHA == nil {
			t.Fatal("PR should have head SHA")
		}

		// Proceed with workflow
		mockAPI.WaitForWorkflowsConclusion = "success"
		_, _ = mockAPI.WaitForWorkflows(5 * time.Minute)
		_ = mockAPI.MergePullRequest(*pr.Number, "squash")
	})
}
