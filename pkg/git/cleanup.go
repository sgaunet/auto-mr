package git

import "fmt"

// CleanupReport tracks the state of each cleanup operation.
type CleanupReport struct {
	// Step completion status
	SwitchedBranch bool
	PulledChanges  bool
	Pruned         bool
	DeletedBranch  bool

	// Errors encountered (nil if step succeeded)
	SwitchError error
	PullError   error
	PruneError  error
	DeleteError error

	// Metadata
	MainBranch  string
	BranchName  string
}

// Success returns true if all critical steps completed successfully.
// Critical steps are: SwitchBranch and Pull.
func (r *CleanupReport) Success() bool {
	return r.SwitchedBranch && r.PulledChanges
}

// PartialSuccess returns true if at least one step completed successfully.
func (r *CleanupReport) PartialSuccess() bool {
	return r.SwitchedBranch || r.PulledChanges || r.Pruned || r.DeletedBranch
}

// FirstError returns the first error encountered, or nil if all succeeded.
// Errors are returned in execution order: Switch -> Pull -> Prune -> Delete.
func (r *CleanupReport) FirstError() error {
	if r.SwitchError != nil {
		return r.SwitchError
	}
	if r.PullError != nil {
		return r.PullError
	}
	if r.PruneError != nil {
		return r.PruneError
	}
	return r.DeleteError
}

// Cleanup performs post-merge cleanup operations and returns a detailed report.
//
// This method implements a hybrid error handling strategy:
//   - Critical operations (switch, pull) fail-fast - stop execution on error
//   - Best-effort operations (prune, delete) continue-on-error - log warning and continue
//
// The hybrid approach ensures that git state is valid (critical operations) while
// allowing recovery from network issues or minor failures (best-effort operations).
func (r *Repository) Cleanup(mainBranch, currentBranch string) *CleanupReport {
	report := &CleanupReport{
		MainBranch: mainBranch,
		BranchName: currentBranch,
	}

	// Step 1: Switch to main branch (CRITICAL - fail-fast)
	if err := r.SwitchBranch(mainBranch); err != nil {
		report.SwitchError = fmt.Errorf(
			"failed to switch to main branch: %w\n\n"+
				"If you have local changes that conflict, please handle them manually:\n"+
				"  - Commit your changes, or\n"+
				"  - Stash your changes with: git stash\n"+
				"  - Then run: git switch %s",
			err, mainBranch)
		return report // Stop - can't proceed without valid branch state
	}
	report.SwitchedBranch = true

	// Step 2: Pull latest changes (CRITICAL - fail-fast)
	if err := r.Pull(); err != nil {
		report.PullError = fmt.Errorf(
			"failed to pull changes: %w\n\n"+
				"Please resolve any conflicts manually and run: git pull",
			err)
		return report // Stop - can't proceed without up-to-date branch
	}
	report.PulledChanges = true

	// Step 3: Fetch and prune (BEST-EFFORT - continue on error)
	if err := r.FetchAndPrune(); err != nil {
		report.PruneError = fmt.Errorf(
			"failed to fetch and prune: %w\n\n"+
				"You can manually run: git fetch --prune",
			err)
		r.log.Warn("Fetch and prune failed, continuing with cleanup")
	} else {
		report.Pruned = true
	}

	// Step 4: Delete feature branch (BEST-EFFORT - continue on error)
	if err := r.DeleteBranch(currentBranch); err != nil {
		report.DeleteError = fmt.Errorf(
			"failed to delete branch: %w\n\n"+
				"You can manually delete it with: git branch -D %s",
			err, currentBranch)
		r.log.Warn("Branch deletion failed, but cleanup is complete")
	} else {
		report.DeletedBranch = true
	}

	return report
}
