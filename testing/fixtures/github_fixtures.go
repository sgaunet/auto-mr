// Package fixtures provides common test data structures for testing.
package fixtures

import (
	"time"

	"github.com/google/go-github/v69/github"
	ghpkg "github.com/sgaunet/auto-mr/pkg/github"
)

// Test constants for GitHub fixtures.
const (
	defaultPRNumber           = 123
	successfulCheckDuration   = 5 * time.Minute
	failedCheckDuration       = 3 * time.Minute
	defaultWorkflowJobMinutes = 5
)

// GitHub fixtures for common test scenarios

// ValidPullRequest returns a valid GitHub pull request for testing.
func ValidPullRequest() *github.PullRequest {
	return &github.PullRequest{
		Number: github.Ptr(defaultPRNumber),
		Title:  github.Ptr("Test Pull Request"),
		State:  github.Ptr("open"),
		Head: &github.PullRequestBranch{
			Ref: github.Ptr("feature-branch"),
			SHA: github.Ptr("abc123def456"),
		},
		Base: &github.PullRequestBranch{
			Ref: github.Ptr("main"),
		},
		User: &github.User{
			Login: github.Ptr("testuser"),
		},
		HTMLURL: github.Ptr("https://github.com/owner/repo/pull/123"),
	}
}

// SuccessfulCheckRun returns a successful GitHub check run for testing.
func SuccessfulCheckRun(id int64, name string) *github.CheckRun {
	now := time.Now()
	completed := now.Add(successfulCheckDuration)
	return &github.CheckRun{
		ID:         github.Ptr(id),
		Name:       github.Ptr(name),
		Status:     github.Ptr("completed"),
		Conclusion: github.Ptr("success"),
		StartedAt:  &github.Timestamp{Time: now},
		CompletedAt: &github.Timestamp{Time: completed},
		HTMLURL:    github.Ptr("https://github.com/owner/repo/runs/123"),
	}
}

// FailedCheckRun returns a failed GitHub check run for testing.
func FailedCheckRun(id int64, name string) *github.CheckRun {
	now := time.Now()
	completed := now.Add(failedCheckDuration)
	return &github.CheckRun{
		ID:         github.Ptr(id),
		Name:       github.Ptr(name),
		Status:     github.Ptr("completed"),
		Conclusion: github.Ptr("failure"),
		StartedAt:  &github.Timestamp{Time: now},
		CompletedAt: &github.Timestamp{Time: completed},
		HTMLURL:    github.Ptr("https://github.com/owner/repo/runs/456"),
	}
}

// RunningCheckRun returns a running GitHub check run for testing.
func RunningCheckRun(id int64, name string) *github.CheckRun {
	now := time.Now()
	return &github.CheckRun{
		ID:        github.Ptr(id),
		Name:      github.Ptr(name),
		Status:    github.Ptr("in_progress"),
		StartedAt: &github.Timestamp{Time: now},
		HTMLURL:   github.Ptr("https://github.com/owner/repo/runs/789"),
	}
}

// QueuedCheckRun returns a queued GitHub check run for testing.
func QueuedCheckRun(id int64, name string) *github.CheckRun {
	return &github.CheckRun{
		ID:      github.Ptr(id),
		Name:    github.Ptr(name),
		Status:  github.Ptr("queued"),
		HTMLURL: github.Ptr("https://github.com/owner/repo/runs/101"),
	}
}

// SkippedCheckRun returns a skipped GitHub check run for testing.
func SkippedCheckRun(id int64, name string) *github.CheckRun {
	now := time.Now()
	completed := now.Add(1 * time.Minute)
	return &github.CheckRun{
		ID:         github.Ptr(id),
		Name:       github.Ptr(name),
		Status:     github.Ptr("completed"),
		Conclusion: github.Ptr("skipped"),
		StartedAt:  &github.Timestamp{Time: now},
		CompletedAt: &github.Timestamp{Time: completed},
		HTMLURL:    github.Ptr("https://github.com/owner/repo/runs/202"),
	}
}

// JobInfoFromCheckRun converts a GitHub CheckRun to JobInfo for testing.
func JobInfoFromCheckRun(check *github.CheckRun) *ghpkg.JobInfo {
	job := &ghpkg.JobInfo{
		ID:         check.GetID(),
		Name:       check.GetName(),
		Status:     check.GetStatus(),
		Conclusion: check.GetConclusion(),
		HTMLURL:    check.GetHTMLURL(),
	}

	if check.StartedAt != nil {
		job.StartedAt = check.StartedAt.GetTime()
	}
	if check.CompletedAt != nil {
		job.CompletedAt = check.CompletedAt.GetTime()
	}

	return job
}

// ValidLabels returns a list of valid GitHub labels for testing.
func ValidLabels() []*ghpkg.Label {
	return []*ghpkg.Label{
		{Name: "bug"},
		{Name: "enhancement"},
		{Name: "documentation"},
		{Name: "help wanted"},
	}
}

// WorkflowRun returns a GitHub workflow run for testing.
func WorkflowRun(id int64, status, conclusion string) *github.WorkflowRun {
	return &github.WorkflowRun{
		ID:         github.Ptr(id),
		Status:     github.Ptr(status),
		Conclusion: github.Ptr(conclusion),
		HeadSHA:    github.Ptr("abc123def456"),
	}
}

// WorkflowJob returns a GitHub workflow job for testing.
func WorkflowJob(id int64, name, status, conclusion string) *github.WorkflowJob {
	now := time.Now()
	job := &github.WorkflowJob{
		ID:     github.Ptr(id),
		Name:   github.Ptr(name),
		Status: github.Ptr(status),
	}

	if conclusion != "" {
		job.Conclusion = github.Ptr(conclusion)
		completed := now.Add(defaultWorkflowJobMinutes * time.Minute)
		job.StartedAt = &github.Timestamp{Time: now}
		job.CompletedAt = &github.Timestamp{Time: completed}
	} else if status == "in_progress" {
		job.StartedAt = &github.Timestamp{Time: now}
	}

	return job
}
