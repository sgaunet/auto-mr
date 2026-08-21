package git_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/pkg/git"
)

// headBranch reports the branch currently checked out in dir.
func headBranch(t *testing.T, dir string) string {
	t.Helper()
	out, err := gitCmd(dir, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to read HEAD in %s: %v\n%s", dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestNativeGitCommands_IgnoreInheritedRepoLocalEnv checks that the native git
// commands act on the repository auto-mr was opened against, never on one named
// by an inherited environment variable.
//
// Git exports GIT_DIR, GIT_INDEX_FILE and GIT_WORK_TREE for the repository a hook
// fires in, so an auto-mr started from a hook — or from a shell where the user
// exported GIT_DIR — would otherwise switch, delete and push in the wrong place
// even though cmd.Dir names the right one.
func TestNativeGitCommands_IgnoreInheritedRepoLocalEnv(t *testing.T) {
	target := newRepoWithBranches(t, "feature-env", "doomed")
	other := newRepoWithBranches(t)

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
	t.Setenv("GIT_WORK_TREE", other)

	repo, err := git.OpenRepository(target)
	if err != nil {
		t.Fatalf("Failed to open target repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := repo.SwitchBranch(ctx, "feature-env"); err != nil {
		t.Fatalf("SwitchBranch honoured inherited repository-local git env: %v", err)
	}
	if got := headBranch(t, target); got != "feature-env" {
		t.Errorf("target repository HEAD = %q, want %q", got, "feature-env")
	}

	if err := repo.DeleteBranch(ctx, "doomed"); err != nil {
		t.Fatalf("DeleteBranch honoured inherited repository-local git env: %v", err)
	}
	if out, err := gitCmd(target, "rev-parse", "--verify", "doomed").CombinedOutput(); err == nil {
		t.Errorf("branch 'doomed' still present in target repository: %s", out)
	}

	// The repository named by the inherited variables must be untouched.
	if got := headBranch(t, other); got != "master" {
		t.Errorf("unrelated repository was modified: HEAD = %q, want %q", got, "master")
	}
}
