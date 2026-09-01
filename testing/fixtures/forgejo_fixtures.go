package fixtures

import (
	"code.gitea.io/sdk/gitea"
	fjpkg "github.com/sgaunet/auto-mr/pkg/forgejo"
)

const (
	defaultPRIndex = int64(42)
	defaultHeadRef = "feature-branch"
)

// ValidForgejoPullRequest returns a realistic Forgejo pull request for testing.
func ValidForgejoPullRequest() *gitea.PullRequest {
	return &gitea.PullRequest{
		Index:   defaultPRIndex,
		Title:   "Test Pull Request",
		State:   gitea.StateOpen,
		HTMLURL: "https://forgejo.example.com/owner/repo/pulls/42",
		Head: &gitea.PRBranchInfo{
			Ref: defaultHeadRef,
			Sha: TestCommitSHA,
		},
		Base: &gitea.PRBranchInfo{
			Ref: "main",
		},
	}
}

// ValidForgejoLabels returns a set of Forgejo labels for testing.
func ValidForgejoLabels() []fjpkg.Label {
	return []fjpkg.Label{
		{Name: labelBug},
		{Name: labelEnhancement},
	}
}

// ForgejoCommitStatus returns a single commit status in the given state.
func ForgejoCommitStatus(context string, state gitea.StatusState) *gitea.Status {
	return &gitea.Status{
		Context:     context,
		State:       state,
		TargetURL:   "https://forgejo.example.com/owner/repo/actions/runs/1",
		Description: "check " + context,
	}
}

// ForgejoCombinedStatus returns a combined status aggregating the given statuses.
// The overall state mirrors what a Forgejo server reports for that set.
func ForgejoCombinedStatus(state gitea.StatusState, statuses ...*gitea.Status) *gitea.CombinedStatus {
	return &gitea.CombinedStatus{
		State:      state,
		SHA:        TestCommitSHA,
		TotalCount: len(statuses),
		Statuses:   statuses,
	}
}
