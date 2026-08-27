package git_test

import (
	"errors"
	"strings"
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
				PullError:   errPull,
				PruneError:  errPrune,
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
			if !errors.Is(got, tc.expectError) {
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
	// Run cleanup against a repository whose main branch does not exist, so the very
	// first step fails and the report carries the recovery guidance for it.
	//
	// The previous version of this test logged the text it expected and then asserted
	// only that a freshly allocated report was non-nil, so it passed regardless of
	// what Cleanup produced. The recovery instructions are the user's only route out
	// of a half-finished cleanup, so they are worth asserting for real.
	repoDir := newRepoWithBranches(t, "main")
	repo, err := git.OpenRepository(repoDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}

	report := repo.Cleanup(t.Context(), "no-such-main-branch", "feature")
	if report == nil {
		t.Fatal("Cleanup returned a nil report")
	}

	if report.SwitchError == nil {
		t.Fatal("expected a switch error when the target branch does not exist")
	}
	if report.SwitchedBranch {
		t.Error("report claims the branch was switched even though switching failed")
	}

	// Switching is a critical step, so cleanup must stop rather than press on.
	if report.PulledChanges || report.Pruned || report.DeletedBranch {
		t.Error("cleanup continued past a failed switch; later steps must not run")
	}
	if report.Success() {
		t.Error("Success() is true for a cleanup whose first step failed")
	}

	// The guidance must name the commands that get the user unstuck.
	msg := report.SwitchError.Error()
	for _, want := range []string{"git stash", "git switch"} {
		if !strings.Contains(msg, want) {
			t.Errorf("switch error does not mention %q; got:\n%s", want, msg)
		}
	}
	if report.FirstError() == nil {
		t.Error("FirstError() returned nil despite a failed step")
	}
}
