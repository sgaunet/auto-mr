// Package platform provides a unified abstraction layer for GitLab and GitHub operations.
package platform

// Label represents a platform-agnostic label.
type Label struct {
	Name string
}

// MergeRequest represents a platform-agnostic merge/pull request.
type MergeRequest struct {
	ID           int64  // GitLab: MR IID; GitHub: PR Number
	WebURL       string // Browser URL
	SourceBranch string // Needed for GitHub post-merge branch deletion
}

// CreateParams holds parameters for creating a merge/pull request.
// Assignees and reviewers are not included here; they come from the
// config stored in each adapter at construction time.
type CreateParams struct {
	SourceBranch string
	TargetBranch string
	Title        string
	Body         string
	Labels       []string
	Squash       bool
}

// MergeParams holds parameters for merging a merge/pull request.
type MergeParams struct {
	MRID         int64
	Squash       bool
	CommitTitle  string
	SourceBranch string // GitHub: for branch deletion; GitLab: unused
}
