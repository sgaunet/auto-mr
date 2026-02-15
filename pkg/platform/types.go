// Package platform provides a unified abstraction layer for GitLab and GitHub operations.
//
// The [Provider] interface defines a common API for merge/pull request lifecycle
// operations that both platforms implement. This allows the main application logic
// to be platform-agnostic.
//
// Use [NewProvider] to create the appropriate adapter based on the detected platform:
//
//	provider, err := platform.NewProvider(git.PlatformGitHub, cfg, logger)
//	provider.Initialize(remoteURL)
//	mr, _ := provider.Create(platform.CreateParams{...})
//	status, _ := provider.WaitForPipeline(30 * time.Minute)
//	provider.Merge(platform.MergeParams{MRID: mr.ID, ...})
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
