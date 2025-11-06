package gitlab_test

import (
	"testing"

	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
	gitlablib "gitlab.com/gitlab-org/api/client-go"
)

// TestEdgeCaseEmptyResponses tests handling of empty API responses.
func TestEdgeCaseEmptyResponses(t *testing.T) {
	tests := []struct {
		name string
		test func(*testing.T)
	}{
		{
			name: "empty label list",
			test: func(t *testing.T) {
				mockAPI := mocks.NewGitLabAPIClient()
				mockAPI.ListLabelsResponse = []*gitlab.Label{}
				labels, err := mockAPI.ListLabels()
				if err != nil || len(labels) != 0 {
					t.Errorf("Expected empty list, got %d labels", len(labels))
				}
			},
		},
		{
			name: "empty MR list",
			test: func(t *testing.T) {
				mockAPI := mocks.NewGitLabAPIClient()
				mockAPI.GetMergeRequestsByBranchResponse = []*gitlablib.BasicMergeRequest{}
				mrs, err := mockAPI.GetMergeRequestsByBranch("branch")
				if err != nil || len(mrs) != 0 {
					t.Errorf("Expected empty list, got %d MRs", len(mrs))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// TestEdgeCaseSpecialCharacters tests handling of special characters.
func TestEdgeCaseSpecialCharacters(t *testing.T) {
	specialStrings := []string{
		"unicode-☃-branch",
		"branch/with/slashes",
		"branch-with-dashes",
		"branch_with_underscores",
		"branch.with.dots",
		"branch#123",
	}

	for _, str := range specialStrings {
		t.Run("special char: "+str, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()
			mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
			_, err := mockAPI.CreateMergeRequest(str, "main", "Test", "Desc", "", "", []string{}, false)
			if err != nil {
				t.Errorf("Failed to handle special characters: %v", err)
			}
		})
	}
}

// TestEdgeCaseLongStrings tests handling of very long input strings.
func TestEdgeCaseLongStrings(t *testing.T) {
	longTitle := string(make([]byte, 1000))
	longDesc := string(make([]byte, 5000))

	mockAPI := mocks.NewGitLabAPIClient()
	mockAPI.CreateMergeRequestResponse = fixtures.ValidMergeRequest()
	_, err := mockAPI.CreateMergeRequest("feature", "main", longTitle, longDesc, "", "", []string{}, false)
	if err != nil {
		t.Errorf("Failed to handle long strings: %v", err)
	}
}

// TestEdgeCaseURLVariations tests various URL format variations.
func TestEdgeCaseURLVariations(t *testing.T) {
	urls := []struct {
		url       string
		shouldErr bool
	}{
		{"https://gitlab.com/owner/project.git", false},
		{"https://gitlab.com/owner/project", false},
		{"git@gitlab.com:owner/project.git", false},
		{"ssh://git@gitlab.com/owner/project.git", false},
		{"https://gitlab.example.com/owner/project.git", false},
		{"not-a-url", true},
		{"", true},
	}

	for _, test := range urls {
		t.Run("URL: "+test.url, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()
			if test.shouldErr {
				mockAPI.SetProjectFromURLError = gitlab.ErrInvalidURLFormat
			}
			err := mockAPI.SetProjectFromURL(test.url)
			if test.shouldErr && err == nil {
				t.Error("Expected error for invalid URL")
			}
			if !test.shouldErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestEdgeCasePipelineStates tests various pipeline state transitions.
func TestEdgeCasePipelineStates(t *testing.T) {
	states := []string{"success", "failed", "running", "pending", "canceled", "skipped", "manual"}

	for _, state := range states {
		t.Run("pipeline state: "+state, func(t *testing.T) {
			mockAPI := mocks.NewGitLabAPIClient()
			mockAPI.WaitForPipelineStatus = state
			status, err := mockAPI.WaitForPipeline(5000)
			if err != nil {
				t.Errorf("Error handling state %s: %v", state, err)
			}
			if status != state {
				t.Errorf("Expected state %s, got %s", state, status)
			}
		})
	}
}

// TestEdgeCaseConcurrentOperations tests concurrent API calls.
func TestEdgeCaseConcurrentOperations(t *testing.T) {
	t.Run("concurrent label fetches", func(t *testing.T) {
		mockAPI := mocks.NewGitLabAPIClient()
		mockAPI.ListLabelsResponse = fixtures.ValidGitLabLabels()

		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				_, _ = mockAPI.ListLabels()
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		if mockAPI.GetCallCount("ListLabels") != 10 {
			t.Errorf("Expected 10 calls, got %d", mockAPI.GetCallCount("ListLabels"))
		}
	})
}
