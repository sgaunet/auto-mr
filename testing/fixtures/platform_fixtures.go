package fixtures

import "github.com/sgaunet/auto-mr/pkg/platform"

// Test constants for platform fixtures.
const (
	defaultMRID     = 42
	defaultWebURL   = "https://example.com/owner/repo/-/merge_requests/42"
	defaultSourceBr = "feature-branch"
	defaultTargetBr = "main"
	defaultTitle    = "Test merge request"
	defaultBody     = "Test body"
)

// Shared label names used across GitHub, GitLab, and platform fixtures.
const (
	labelBug           = "bug"
	labelEnhancement   = "enhancement"
	labelDocumentation = "documentation"
	labelHelpWanted    = "help wanted"
)

// ValidPlatformMergeRequest returns a valid platform MergeRequest for testing.
func ValidPlatformMergeRequest() *platform.MergeRequest {
	return &platform.MergeRequest{
		ID:           defaultMRID,
		WebURL:       defaultWebURL,
		SourceBranch: defaultSourceBr,
	}
}

// ValidPlatformLabels returns a list of valid platform labels for testing.
func ValidPlatformLabels() []platform.Label {
	return []platform.Label{
		{Name: labelBug},
		{Name: labelEnhancement},
		{Name: labelDocumentation},
		{Name: labelHelpWanted},
	}
}

// ValidCreateParams returns valid CreateParams for testing.
func ValidCreateParams() platform.CreateParams {
	return platform.CreateParams{
		SourceBranch: defaultSourceBr,
		TargetBranch: defaultTargetBr,
		Title:        defaultTitle,
		Body:         defaultBody,
		Labels:       []string{labelBug},
		Squash:       true,
	}
}

// ValidMergeParams returns valid MergeParams for testing.
func ValidMergeParams() platform.MergeParams {
	return platform.MergeParams{
		MRID:         defaultMRID,
		Squash:       true,
		CommitTitle:  defaultTitle,
		SourceBranch: defaultSourceBr,
	}
}
