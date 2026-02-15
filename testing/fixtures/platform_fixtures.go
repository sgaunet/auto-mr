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
		{Name: "bug"},
		{Name: "enhancement"},
		{Name: "documentation"},
		{Name: "help wanted"},
	}
}

// ValidCreateParams returns valid CreateParams for testing.
func ValidCreateParams() platform.CreateParams {
	return platform.CreateParams{
		SourceBranch: defaultSourceBr,
		TargetBranch: defaultTargetBr,
		Title:        defaultTitle,
		Body:         defaultBody,
		Labels:       []string{"bug"},
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
