package gitlab_test

import (
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
	gitlablib "gitlab.com/gitlab-org/api/client-go"
)

// TestClientConstructor tests the NewClient function.
func TestClientConstructor(t *testing.T) {
	t.Run("NewClient requires GITLAB_TOKEN", func(t *testing.T) {
		t.Skip("Requires environment manipulation")
	})
}

// TestSetProjectFromURL tests the SetProjectFromURL method with various URL formats.
func TestSetProjectFromURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantError bool
	}{
		{
			name:      "valid HTTPS URL",
			url:       "https://gitlab.com/owner/project.git",
			wantError: false,
		},
		{
			name:      "valid HTTPS URL without .git",
			url:       "https://gitlab.com/owner/project",
			wantError: false,
		},
		{
			name:      "valid SSH URL",
			url:       "git@gitlab.com:owner/project.git",
			wantError: false,
		},
		{
			name:      "valid SSH URL without .git",
			url:       "git@gitlab.com:owner/project",
			wantError: false,
		},
		{
			name:      "ssh:// protocol URL",
			url:       "ssh://git@gitlab.com/owner/project.git",
			wantError: false,
		},
		{
			name:      "invalid URL format",
			url:       "not-a-url",
			wantError: true,
		},
		{
			name:      "empty URL",
			url:       "",
			wantError: true,
		},
		{
			name:      "URL with only owner",
			url:       "https://gitlab.com/owner",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()
			if tt.wantError {
				mockAPI.SetProjectFromURLError = gitlab.ErrInvalidURLFormat
			}

			err := mockAPI.SetProjectFromURL(tt.url)

			if tt.wantError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify call was tracked
			if mockAPI.GetCallCount("SetProjectFromURL") != 1 {
				t.Error("Expected SetProjectFromURL to be called once")
			}
		})
	}
}

// TestListLabels tests the ListLabels method.
func TestListLabels(t *testing.T) {
	t.Run("successful label retrieval", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.ListLabelsResponse = fixtures.ValidGitLabLabels()

		labels, err := mockAPI.ListLabels()
		if err != nil {
			t.Fatalf("Failed to list labels: %v", err)
		}

		if len(labels) != 4 {
			t.Errorf("Expected 4 labels, got %d", len(labels))
		}

		// Verify call was tracked
		if mockAPI.GetCallCount("ListLabels") != 1 {
			t.Error("Expected ListLabels to be called once")
		}
	})

	t.Run("API error handling", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.ListLabelsError = gitlab.ErrTokenRequired

		_, err := mockAPI.ListLabels()
		if err == nil {
			t.Error("Expected error but got nil")
		}
	})

	t.Run("empty label list", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.ListLabelsResponse = []*gitlab.Label{}

		labels, err := mockAPI.ListLabels()
		if err != nil {
			t.Fatalf("Failed to list labels: %v", err)
		}

		if len(labels) != 0 {
			t.Errorf("Expected 0 labels, got %d", len(labels))
		}
	})
}

// TestCreateMergeRequest tests the CreateMergeRequest method.
func TestCreateMergeRequest(t *testing.T) {
	t.Run("create MR with all fields", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()

		mr, err := mockAPI.CreateMergeRequest(
			"feature", "main", "Test MR", "Description",
			"user1", "reviewer1", []string{"bug"}, false,
		)
		if err != nil {
			t.Fatalf("Failed to create MR: %v", err)
		}
		if mr == nil {
			t.Fatal("Expected MR to be created")
		}

		// Verify call arguments
		lastCall := mockAPI.GetLastCall("CreateMergeRequest")
		if lastCall.Args["sourceBranch"] != "feature" {
			t.Errorf("Expected source branch 'feature', got %v", lastCall.Args["sourceBranch"])
		}
	})

	t.Run("create MR with minimal fields", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()

		mr, err := mockAPI.CreateMergeRequest(
			"feature", "main", "Test MR", "Description",
			"", "", []string{}, false,
		)
		if err != nil {
			t.Fatalf("Failed to create MR: %v", err)
		}
		if mr == nil {
			t.Fatal("Expected MR to be created")
		}
	})

	t.Run("API error during MR creation", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.CreateMergeRequestError = gitlab.ErrInvalidURLFormat

		_, err := mockAPI.CreateMergeRequest(
			"feature", "main", "Test MR", "Description",
			"", "", []string{}, false,
		)
		if err == nil {
			t.Error("Expected error but got nil")
		}
	})
}

// TestGetMergeRequestByBranch tests the GetMergeRequestByBranch method.
func TestGetMergeRequestByBranch(t *testing.T) {
	t.Run("find existing MR", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.GetMergeRequestByBranchResponse = fixtures.ValidMergeRequest()

		mr, err := mockAPI.GetMergeRequestByBranch("feature", "main")
		if err != nil {
			t.Fatalf("Failed to find MR: %v", err)
		}
		if mr == nil {
			t.Fatal("Expected to find MR")
		}
	})

	t.Run("MR not found", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.GetMergeRequestByBranchError = gitlab.ErrMRNotFound

		_, err := mockAPI.GetMergeRequestByBranch("nonexistent", "main")
		if err == nil {
			t.Error("Expected error for non-existent MR")
		}
	})
}

// TestWaitForPipeline tests the WaitForPipeline method.
func TestWaitForPipeline(t *testing.T) {
	t.Run("pipeline completes successfully", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.WaitForPipelineStatus = "success"

		status, err := mockAPI.WaitForPipeline(5 * time.Minute)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if status != "success" {
			t.Errorf("Expected success status, got %s", status)
		}
	})

	t.Run("pipeline fails", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.WaitForPipelineStatus = "failed"

		status, err := mockAPI.WaitForPipeline(5 * time.Minute)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if status != "failed" {
			t.Errorf("Expected failed status, got %s", status)
		}
	})

	t.Run("pipeline timeout", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.WaitForPipelineError = gitlab.ErrPipelineTimeout

		_, err := mockAPI.WaitForPipeline(1 * time.Millisecond)
		if err == nil {
			t.Error("Expected timeout error")
		}
		if err != gitlab.ErrPipelineTimeout {
			t.Errorf("Expected ErrPipelineTimeout, got %v", err)
		}
	})
}

// TestApproveMergeRequest tests the ApproveMergeRequest method.
func TestApproveMergeRequest(t *testing.T) {
	t.Run("approve MR successfully", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()

		err := mockAPI.ApproveMergeRequest(123)
		if err != nil {
			t.Fatalf("Failed to approve MR: %v", err)
		}

		// Verify call was tracked
		if mockAPI.GetCallCount("ApproveMergeRequest") != 1 {
			t.Error("Expected ApproveMergeRequest to be called once")
		}
	})

	t.Run("approval failure", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.ApproveMergeRequestError = gitlab.ErrTokenRequired

		err := mockAPI.ApproveMergeRequest(123)
		if err == nil {
			t.Error("Expected error but got nil")
		}
	})
}

// TestMergeMergeRequest tests the MergeMergeRequest method.
func TestMergeMergeRequest(t *testing.T) {
	tests := []struct {
		name   string
		squash bool
	}{
		{"merge without squash", false},
		{"merge with squash", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()

			err := mockAPI.MergeMergeRequest(123, tt.squash)
			if err != nil {
				t.Fatalf("Failed to merge MR: %v", err)
			}

			// Verify call arguments
			lastCall := mockAPI.GetLastCall("MergeMergeRequest")
			if lastCall.Args["squash"] != tt.squash {
				t.Errorf("Expected squash %v, got %v", tt.squash, lastCall.Args["squash"])
			}
		})
	}

	t.Run("merge failure", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.MergeMergeRequestError = gitlab.ErrMRNotFound

		err := mockAPI.MergeMergeRequest(123, false)
		if err == nil {
			t.Error("Expected error but got nil")
		}
	})
}

// TestGetMergeRequestsByBranch tests the GetMergeRequestsByBranch method.
func TestGetMergeRequestsByBranch(t *testing.T) {
	t.Run("find MRs for branch", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.GetMergeRequestsByBranchResponse = []*gitlablib.BasicMergeRequest{
			fixtures.BasicMergeRequest(123, "feature-branch", "main"),
		}

		mrs, err := mockAPI.GetMergeRequestsByBranch("feature-branch")
		if err != nil {
			t.Fatalf("Failed to list MRs: %v", err)
		}

		if len(mrs) != 1 {
			t.Errorf("Expected 1 MR, got %d", len(mrs))
		}
	})

	t.Run("no MRs for branch", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.GetMergeRequestsByBranchResponse = []*gitlablib.BasicMergeRequest{}

		mrs, err := mockAPI.GetMergeRequestsByBranch("nonexistent-branch")
		if err != nil {
			t.Fatalf("Failed to list MRs: %v", err)
		}

		if len(mrs) != 0 {
			t.Errorf("Expected 0 MRs, got %d", len(mrs))
		}
	})
}
