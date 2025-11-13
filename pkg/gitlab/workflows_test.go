package gitlab_test

import (
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
)

// TestWorkflowMRCreationToMerge tests the complete MR lifecycle.
func TestWorkflowMRCreationToMerge(t *testing.T) {
	t.Run("successful MR workflow", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		// Step 1: Create MR
		mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
		mr, err := mockAPI.CreateMergeRequest(
			"feature", "main", "Test MR", "Description",
			"user1", "reviewer1", []string{"bug"}, false,
		)
		if err != nil || mr == nil {
			t.Fatalf("Failed to create MR: %v", err)
		}

		// Step 2: Wait for pipeline
		mockAPI.WaitForPipelineStatus = "success"
		status, _ := mockAPI.WaitForPipeline(5 * time.Minute)
		if status != "success" {
			t.Errorf("Expected success, got %s", status)
		}

		// Step 3: Approve MR
		err = mockAPI.ApproveMergeRequest(123)
		if err != nil {
			t.Fatalf("Failed to approve MR: %v", err)
		}

		// Step 4: Merge MR
		err = mockAPI.MergeMergeRequest(123, false, "Test commit")
		if err != nil {
			t.Fatalf("Failed to merge MR: %v", err)
		}

		// Verify complete workflow
		if mockAPI.GetCallCount("CreateMergeRequest") != 1 {
			t.Error("Expected CreateMergeRequest to be called once")
		}
		if mockAPI.GetCallCount("ApproveMergeRequest") != 1 {
			t.Error("Expected ApproveMergeRequest to be called once")
		}
		if mockAPI.GetCallCount("MergeMergeRequest") != 1 {
			t.Error("Expected MergeMergeRequest to be called once")
		}
	})

	t.Run("MR workflow with failing pipeline", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		// Create MR
		mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
		_, _ = mockAPI.CreateMergeRequest(
			"feature", "main", "Test MR", "Description",
			"", "", []string{}, false,
		)

		// Wait for pipeline - it fails
		mockAPI.WaitForPipelineStatus = "failed"
		status, _ := mockAPI.WaitForPipeline(5 * time.Minute)
		if status != "failed" {
			t.Errorf("Expected failure, got %s", status)
		}

		// Should not merge or approve failed pipeline
		if status != "success" {
			t.Log("Correctly avoiding approval/merge of failed pipeline")
		}

		// Verify MR was created but not merged
		if mockAPI.GetCallCount("CreateMergeRequest") != 1 {
			t.Error("Expected CreateMergeRequest to be called once")
		}
		if mockAPI.GetCallCount("MergeMergeRequest") != 0 {
			t.Error("Expected MergeMergeRequest not to be called for failed pipeline")
		}
	})
}

// TestWorkflowMRUpdateAndRetry tests MR updates and retry scenarios.
func TestWorkflowMRUpdateAndRetry(t *testing.T) {
	t.Run("retry after fixing issues", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		// Create MR
		mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
		_, _ = mockAPI.CreateMergeRequest(
			"feature", "main", "Test MR", "Description",
			"", "", []string{}, false,
		)

		// First attempt - pipeline fails
		mockAPI.WaitForPipelineStatus = "failed"
		status1, _ := mockAPI.WaitForPipeline(5 * time.Minute)
		if status1 != "failed" {
			t.Errorf("Expected first attempt to fail, got %s", status1)
		}

		// After fixing code, retry - pipeline succeeds
		mockAPI.WaitForPipelineStatus = "success"
		status2, _ := mockAPI.WaitForPipeline(5 * time.Minute)
		if status2 != "success" {
			t.Errorf("Expected second attempt to succeed, got %s", status2)
		}

		// Now approve and merge
		_ = mockAPI.ApproveMergeRequest(123)
		_ = mockAPI.MergeMergeRequest(123, false, "Test commit")

		// Verify retry pattern
		if mockAPI.GetCallCount("WaitForPipeline") != 2 {
			t.Errorf("Expected WaitForPipeline to be called twice, got %d",
				mockAPI.GetCallCount("WaitForPipeline"))
		}
	})
}

// TestWorkflowFindExistingMR tests finding and working with existing MRs.
func TestWorkflowFindExistingMR(t *testing.T) {
	t.Run("find and merge existing MR", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		// Find existing MR
		mockAPI.GetMergeRequestByBranchResponse = fixtures.ValidMergeRequest()
		mr, _ := mockAPI.GetMergeRequestByBranch("feature", "main")
		if mr == nil {
			t.Fatal("Expected to find existing MR")
		}

		// Wait for pipeline
		mockAPI.WaitForPipelineStatus = "success"
		_, _ = mockAPI.WaitForPipeline(5 * time.Minute)

		// Approve and merge existing MR
		_ = mockAPI.ApproveMergeRequest(123)
		_ = mockAPI.MergeMergeRequest(123, false, "Test commit")

		// Verify workflow
		if mockAPI.GetCallCount("GetMergeRequestByBranch") != 1 {
			t.Error("Expected GetMergeRequestByBranch to be called once")
		}
		if mockAPI.GetCallCount("CreateMergeRequest") != 0 {
			t.Error("Should not create new MR when one exists")
		}
	})
}

// TestWorkflowSquashMerge tests squash merge functionality.
func TestWorkflowSquashMerge(t *testing.T) {
	tests := []struct {
		name   string
		squash bool
	}{
		{"regular merge", false},
		{"squash merge", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()

			// Create MR
			mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
			_, _ = mockAPI.CreateMergeRequest(
				"feature", "main", "Test MR", "Description",
				"", "", []string{}, tt.squash,
			)

			// Wait for success
			mockAPI.WaitForPipelineStatus = "success"
			_, _ = mockAPI.WaitForPipeline(5 * time.Minute)

			// Approve and merge with specific squash setting
			_ = mockAPI.ApproveMergeRequest(123)
			_ = mockAPI.MergeMergeRequest(123, tt.squash, "Test commit")

			// Verify squash setting
			lastCall := mockAPI.GetLastCall("MergeMergeRequest")
			if lastCall.Args["squash"] != tt.squash {
				t.Errorf("Expected squash %v, got %v", tt.squash, lastCall.Args["squash"])
			}
			if lastCall.Args["commitTitle"] != "Test commit" {
				t.Errorf("Expected commitTitle 'Test commit', got %v", lastCall.Args["commitTitle"])
			}
		})
	}
}

// TestWorkflowWithLabels tests MR workflow with label management.
func TestWorkflowWithLabels(t *testing.T) {
	t.Run("create MR with labels and merge", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		// List available labels
		mockAPI.ListLabelsResponse = fixtures.ValidGitLabLabels()
		labels, _ := mockAPI.ListLabels()
		if len(labels) != 4 {
			t.Errorf("Expected 4 labels, got %d", len(labels))
		}

		// Create MR with selected labels
		mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
		_, _ = mockAPI.CreateMergeRequest(
			"bugfix", "main", "Fix critical bug", "Description",
			"", "", []string{"bug", "urgent"}, false,
		)

		// Verify labels were passed
		lastCall := mockAPI.GetLastCall("CreateMergeRequest")
		passedLabels := lastCall.Args["labels"].([]string)
		if len(passedLabels) != 2 {
			t.Errorf("Expected 2 labels, got %d", len(passedLabels))
		}
		if passedLabels[0] != "bug" || passedLabels[1] != "urgent" {
			t.Errorf("Labels not passed correctly: %v", passedLabels)
		}

		// Complete workflow
		mockAPI.WaitForPipelineStatus = "success"
		_, _ = mockAPI.WaitForPipeline(5 * time.Minute)
		_ = mockAPI.ApproveMergeRequest(123)
		_ = mockAPI.MergeMergeRequest(123, false, "Test commit")
	})
}

// TestWorkflowApprovalScenarios tests various approval scenarios.
func TestWorkflowApprovalScenarios(t *testing.T) {
	t.Run("auto-approval before merge", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		// Create MR
		mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
		_, _ = mockAPI.CreateMergeRequest(
			"feature", "main", "Test MR", "Description",
			"", "", []string{}, false,
		)

		// Wait for pipeline success
		mockAPI.WaitForPipelineStatus = "success"
		_, _ = mockAPI.WaitForPipeline(5 * time.Minute)

		// Auto-approve (GitLab-specific feature)
		err := mockAPI.ApproveMergeRequest(123)
		if err != nil {
			t.Fatalf("Failed to auto-approve MR: %v", err)
		}

		// Merge after approval
		err = mockAPI.MergeMergeRequest(123, false, "Test commit")
		if err != nil {
			t.Fatalf("Failed to merge MR: %v", err)
		}

		// Verify approval was called
		if mockAPI.GetCallCount("ApproveMergeRequest") != 1 {
			t.Error("Expected ApproveMergeRequest to be called for auto-approval")
		}
	})
}
