// Package gitlab provides a GitLab API client for merge request lifecycle management.
//
// The package handles:
//   - Creating and fetching merge requests with assignees, reviewers, and labels
//   - Waiting for CI/CD pipeline completion with real-time job-level visualization
//   - Approving and merging merge requests
//   - Label retrieval for interactive selection
//
// Authentication requires a GITLAB_TOKEN environment variable containing a
// personal access token with api scope.
//
// Usage:
//
//	client, err := gitlab.NewClient()
//	client.SetLogger(logger)
//	client.SetProjectFromURL("https://gitlab.com/org/repo.git")
//	labels, _ := client.ListLabels()
//	mr, _ := client.CreateMergeRequest("feature", "main", "Title", "Body", "user", "reviewer", nil, false)
//
// Thread Safety: [Client] is not safe for concurrent use. The pipeline waiting
// methods use internal goroutines for parallel job fetching but the Client itself
// should be used from a single goroutine.
package gitlab
