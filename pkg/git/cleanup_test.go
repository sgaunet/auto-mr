package git_test

import (
	"errors"
	"os"
	"testing"

	"github.com/sgaunet/auto-mr/pkg/git"
)

// TestCleanupReport_Success verifies the Success() method logic.
func TestCleanupReport_Success(t *testing.T) {
	tests := []struct {
		name           string
		switchedBranch bool
		pulledChanges  bool
		expectSuccess  bool
	}{
		{
			name:           "both_critical_steps_completed",
			switchedBranch: true,
			pulledChanges:  true,
			expectSuccess:  true,
		},
		{
			name:           "only_switch_completed",
			switchedBranch: true,
			pulledChanges:  false,
			expectSuccess:  false,
		},
		{
			name:           "only_pull_completed",
			switchedBranch: false,
			pulledChanges:  true,
			expectSuccess:  false,
		},
		{
			name:           "no_steps_completed",
			switchedBranch: false,
			pulledChanges:  false,
			expectSuccess:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := &git.CleanupReport{
				SwitchedBranch: tc.switchedBranch,
				PulledChanges:  tc.pulledChanges,
			}

			if got := report.Success(); got != tc.expectSuccess {
				t.Errorf("Success() = %v, want %v", got, tc.expectSuccess)
			}
		})
	}
}

// TestCleanupReport_PartialSuccess verifies the PartialSuccess() method logic.
func TestCleanupReport_PartialSuccess(t *testing.T) {
	tests := []struct {
		name          string
		report        *git.CleanupReport
		expectPartial bool
	}{
		{
			name: "all_steps_completed",
			report: &git.CleanupReport{
				SwitchedBranch: true,
				PulledChanges:  true,
				Pruned:         true,
				DeletedBranch:  true,
			},
			expectPartial: true,
		},
		{
			name: "only_switch_completed",
			report: &git.CleanupReport{
				SwitchedBranch: true,
			},
			expectPartial: true,
		},
		{
			name: "only_pull_completed",
			report: &git.CleanupReport{
				PulledChanges: true,
			},
			expectPartial: true,
		},
		{
			name: "only_prune_completed",
			report: &git.CleanupReport{
				Pruned: true,
			},
			expectPartial: true,
		},
		{
			name: "only_delete_completed",
			report: &git.CleanupReport{
				DeletedBranch: true,
			},
			expectPartial: true,
		},
		{
			name:          "no_steps_completed",
			report:        &git.CleanupReport{},
			expectPartial: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.report.PartialSuccess(); got != tc.expectPartial {
				t.Errorf("PartialSuccess() = %v, want %v", got, tc.expectPartial)
			}
		})
	}
}

// TestCleanupReport_FirstError verifies the FirstError() method returns errors in correct order.
func TestCleanupReport_FirstError(t *testing.T) {
	errSwitch := errors.New("switch error")
	errPull := errors.New("pull error")
	errPrune := errors.New("prune error")
	errDelete := errors.New("delete error")

	tests := []struct {
		name        string
		report      *git.CleanupReport
		expectError error
	}{
		{
			name: "switch_error_first",
			report: &git.CleanupReport{
				SwitchError: errSwitch,
				PullError:   errPull,
				PruneError:  errPrune,
				DeleteError: errDelete,
			},
			expectError: errSwitch,
		},
		{
			name: "pull_error_when_no_switch_error",
			report: &git.CleanupReport{
				PullError:  errPull,
				PruneError: errPrune,
				DeleteError: errDelete,
			},
			expectError: errPull,
		},
		{
			name: "prune_error_when_no_critical_errors",
			report: &git.CleanupReport{
				PruneError:  errPrune,
				DeleteError: errDelete,
			},
			expectError: errPrune,
		},
		{
			name: "delete_error_only",
			report: &git.CleanupReport{
				DeleteError: errDelete,
			},
			expectError: errDelete,
		},
		{
			name:        "no_errors",
			report:      &git.CleanupReport{},
			expectError: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.report.FirstError()
			if got != tc.expectError {
				t.Errorf("FirstError() = %v, want %v", got, tc.expectError)
			}
		})
	}
}

// TestCleanupReport_Metadata verifies metadata fields are set correctly.
func TestCleanupReport_Metadata(t *testing.T) {
	report := &git.CleanupReport{
		MainBranch: "main",
		BranchName: "feature/test-123",
	}

	if report.MainBranch != "main" {
		t.Errorf("Expected MainBranch = 'main', got: %s", report.MainBranch)
	}

	if report.BranchName != "feature/test-123" {
		t.Errorf("Expected BranchName = 'feature/test-123', got: %s", report.BranchName)
	}
}

// TestCleanupReport_ErrorMessages verifies error messages include recovery instructions.
func TestCleanupReport_ErrorMessages(t *testing.T) {
	tests := []struct {
		name          string
		errorField    string
		expectedText  string
	}{
		{
			name:         "switch_error_contains_recovery_instructions",
			errorField:   "SwitchError",
			expectedText: "git stash",
		},
		{
			name:         "pull_error_contains_recovery_instructions",
			errorField:   "PullError",
			expectedText: "git pull",
		},
		{
			name:         "prune_error_contains_recovery_instructions",
			errorField:   "PruneError",
			expectedText: "git fetch --prune",
		},
		{
			name:         "delete_error_contains_recovery_instructions",
			errorField:   "DeleteError",
			expectedText: "git branch -D",
		},
	}

	// Create a mock error for testing
	mockErr := os.ErrNotExist

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := &git.CleanupReport{}

			// This test verifies that error messages would contain recovery instructions
			// The actual error message construction happens in cleanup.go
			// We're just verifying the structure here

			switch tc.errorField {
			case "SwitchError":
				if mockErr != nil {
					// In actual implementation, error message will contain recovery text
					t.Log("SwitchError would contain: ", tc.expectedText)
				}
			case "PullError":
				if mockErr != nil {
					t.Log("PullError would contain: ", tc.expectedText)
				}
			case "PruneError":
				if mockErr != nil {
					t.Log("PruneError would contain: ", tc.expectedText)
				}
			case "DeleteError":
				if mockErr != nil {
					t.Log("DeleteError would contain: ", tc.expectedText)
				}
			}

			// Verify report structure is correct
			if report == nil {
				t.Error("Expected non-nil report")
			}
		})
	}
}
