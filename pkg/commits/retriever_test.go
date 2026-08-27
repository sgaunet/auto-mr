package commits_test

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sgaunet/auto-mr/pkg/commits"
)

// The retriever walks a real go-git history, so these tests build throwaway
// repositories rather than mocking anything. Previously every Retriever method sat at
// zero coverage: the suite tested the Commit and CommitList value types via fixtures
// and never ran the code that produces them.

// testRepo is a throwaway repository with a worktree, used to build history.
type testRepo struct {
	t    *testing.T
	repo *gogit.Repository
	wt   *gogit.Worktree
	dir  string
}

// newTestRepo initialises an empty repository in a temporary directory.
//
// The commit signature is fixed rather than taken from the environment so the tests
// do not depend on the developer's git identity, and the repository is created from
// scratch so nothing can touch the real checkout.
func newTestRepo(t *testing.T) *testRepo {
	t.Helper()

	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	return &testRepo{t: t, repo: repo, wt: wt, dir: dir}
}

// commit writes a file and commits it with the given message.
func (tr *testRepo) commit(message string) {
	tr.t.Helper()

	name := fmt.Sprintf("file-%d.txt", time.Now().UnixNano())
	path := filepath.Join(tr.dir, name)
	if err := os.WriteFile(path, []byte(message), 0o600); err != nil {
		tr.t.Fatalf("write file: %v", err)
	}
	if _, err := tr.wt.Add(name); err != nil {
		tr.t.Fatalf("add %s: %v", name, err)
	}

	_, err := tr.wt.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		tr.t.Fatalf("commit %q: %v", message, err)
	}
}

// branch creates a branch at the current HEAD and checks it out.
func (tr *testRepo) branch(name string) {
	tr.t.Helper()

	if err := tr.wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
		Create: true,
	}); err != nil {
		tr.t.Fatalf("checkout -b %s: %v", name, err)
	}
}

// currentBranch reports the branch HEAD points at.
func (tr *testRepo) currentBranch() string {
	tr.t.Helper()

	head, err := tr.repo.Head()
	if err != nil {
		tr.t.Fatalf("Head: %v", err)
	}
	return head.Name().Short()
}

func TestGetCommits_ReturnsHistoryNewestFirst(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("first commit")
	tr.commit("second commit")
	tr.commit("third commit")

	got, err := commits.NewRetriever(tr.repo).GetCommits(tr.currentBranch())
	if err != nil {
		t.Fatalf("GetCommits: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("got %d commits, want 3", len(got))
	}
	// go-git walks history from the tip backwards.
	if got[0].Title != "third commit" {
		t.Errorf("first result = %q, want the most recent commit", got[0].Title)
	}
	if got[2].Title != "first commit" {
		t.Errorf("last result = %q, want the oldest commit", got[2].Title)
	}
}

func TestGetCommits_UnknownBranchIsAnError(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("first commit")

	if _, err := commits.NewRetriever(tr.repo).GetCommits("no-such-branch"); err == nil {
		t.Error("expected an error for a branch that does not exist")
	}
}

func TestGetCommits_ParsesTitleAndBody(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("feat: add a feature\n\nThe body explains why.\nSecond line.")

	got, err := commits.NewRetriever(tr.repo).GetCommits(tr.currentBranch())
	if err != nil {
		t.Fatalf("GetCommits: %v", err)
	}

	if got[0].Title != "feat: add a feature" {
		t.Errorf("Title = %q", got[0].Title)
	}
	if got[0].Body == "" {
		t.Error("Body is empty; the message body was not parsed")
	}
}

func TestGetCommitsSinceBranch_StopsAtDivergencePoint(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("base one")
	tr.commit("base two")
	base := tr.currentBranch()

	tr.branch("feature")
	tr.commit("feature one")
	tr.commit("feature two")
	tr.commit("feature three")

	got, err := commits.NewRetriever(tr.repo).GetCommitsSinceBranch("feature", base)
	if err != nil {
		t.Fatalf("GetCommitsSinceBranch: %v", err)
	}

	// Only the commits unique to the feature branch belong in a merge request; the
	// two base commits must not be included.
	if len(got) != 3 {
		t.Fatalf("got %d commits, want the 3 unique to the branch: %+v", len(got), titles(got))
	}
	for _, c := range got {
		if c.Title == "base one" || c.Title == "base two" {
			t.Errorf("history from the base branch leaked in: %q", c.Title)
		}
	}
}

func TestGetCommitsSinceBranch_NoDivergenceYieldsNothing(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("base one")
	base := tr.currentBranch()
	tr.branch("feature") // No commits of its own.

	got, err := commits.NewRetriever(tr.repo).GetCommitsSinceBranch("feature", base)
	if err != nil && !errors.Is(err, commits.ErrNoCommits) {
		t.Fatalf("GetCommitsSinceBranch: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d commits for a branch with no unique history: %+v", len(got), titles(got))
	}
}

func TestGetCommitsSinceBranch_UnknownBranchIsAnError(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("base one")
	base := tr.currentBranch()

	if _, err := commits.NewRetriever(tr.repo).GetCommitsSinceBranch("no-such-branch", base); err == nil {
		t.Error("expected an error for a source branch that does not exist")
	}
	if _, err := commits.NewRetriever(tr.repo).GetCommitsSinceBranch(base, "no-such-base"); err == nil {
		t.Error("expected an error for a base branch that does not exist")
	}
}

// TestGetMessageForMR_ManualOverrideWins covers the top of the priority chain: an
// explicit --msg must be used regardless of the branch's history.
func TestGetMessageForMR_ManualOverrideWins(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("base one")
	base := tr.currentBranch()
	tr.branch("feature")
	tr.commit("feature one")
	tr.commit("feature two") // More than one, which would otherwise need a choice.

	selection, err := commits.NewRetriever(tr.repo).GetMessageForMR("feature", base, "explicit title")
	if err != nil {
		t.Fatalf("GetMessageForMR: %v", err)
	}
	if selection.Title != "explicit title" {
		t.Errorf("Title = %q, want the manual override", selection.Title)
	}
}

// TestGetMessageForMR_SingleCommitIsSelectedAutomatically covers the case that needs
// no interaction.
func TestGetMessageForMR_SingleCommitIsSelectedAutomatically(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("base one")
	base := tr.currentBranch()
	tr.branch("feature")
	tr.commit("feat: the only change")

	selection, err := commits.NewRetriever(tr.repo).GetMessageForMR("feature", base, "")
	if err != nil {
		t.Fatalf("GetMessageForMR: %v", err)
	}
	if selection.Title != "feat: the only change" {
		t.Errorf("Title = %q, want the single commit's subject", selection.Title)
	}
}

// TestGetMessageForMR_MultipleCommitsRequireSelection covers the sentinel that sends
// main.go into the interactive prompt.
func TestGetMessageForMR_MultipleCommitsRequireSelection(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("base one")
	base := tr.currentBranch()
	tr.branch("feature")
	tr.commit("feat: first change")
	tr.commit("feat: second change")

	_, err := commits.NewRetriever(tr.repo).GetMessageForMR("feature", base, "")
	if !errors.Is(err, commits.ErrMultipleCommitsFound) {
		t.Errorf("got %v, want ErrMultipleCommitsFound", err)
	}
}

// TestGetMessageForMR_NoCommitsIsAnError covers a branch with nothing to describe.
func TestGetMessageForMR_NoCommitsIsAnError(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("base one")
	base := tr.currentBranch()
	tr.branch("feature")

	if _, err := commits.NewRetriever(tr.repo).GetMessageForMR("feature", base, ""); err == nil {
		t.Error("expected an error for a branch with no commits of its own")
	}
}

// TestSetLogger_IsAccepted checks the setter does not disturb retrieval; the
// retriever defaults to slog.Default() otherwise.
func TestSetLogger_IsAccepted(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("first commit")

	retriever := commits.NewRetriever(tr.repo)
	retriever.SetLogger(slog.New(slog.DiscardHandler))

	if _, err := retriever.GetCommits(tr.currentBranch()); err != nil {
		t.Fatalf("GetCommits after SetLogger: %v", err)
	}
}

func titles(cs []commits.Commit) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Title
	}
	return out
}
