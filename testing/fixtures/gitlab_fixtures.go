// Package fixtures provides common test data structures for testing.
package fixtures

import (
	"time"

	glpkg "github.com/sgaunet/auto-mr/pkg/gitlab"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Test constants for GitLab fixtures.
const (
	defaultMRIID                 = 123
	successfulPipelineDuration   = 10 * time.Minute
	failedPipelineDuration       = 8 * time.Minute
	successfulJobDuration        = 5 * time.Minute
	successfulJobDurationSeconds = 300.0 // 5 minutes
	failedJobDuration            = 3 * time.Minute
	failedJobDurationSeconds     = 180.0 // 3 minutes
	jobID2                       = 2
	jobID3                       = 3
	jobID4                       = 4
)

// GitLab fixtures for common test scenarios

// ValidMergeRequest returns a valid GitLab merge request for testing.
func ValidMergeRequest() *gitlab.MergeRequest {
	return &gitlab.MergeRequest{
		BasicMergeRequest: gitlab.BasicMergeRequest{
			IID:          defaultMRIID,
			Title:        "Test Merge Request",
			State:        "opened",
			SourceBranch: "feature-branch",
			TargetBranch: "main",
			SHA:          "abc123def456",
			Author: &gitlab.BasicUser{
				Username: "testuser",
			},
			WebURL: "https://gitlab.com/owner/project/-/merge_requests/123",
		},
	}
}

// SuccessfulPipeline returns a successful GitLab pipeline for testing.
func SuccessfulPipeline(id int, sha string) *gitlab.PipelineInfo {
	now := time.Now()
	updated := now.Add(successfulPipelineDuration)
	return &gitlab.PipelineInfo{
		ID:        id,
		SHA:       sha,
		Status:    "success",
		Ref:       "feature-branch",
		CreatedAt: &now,
		UpdatedAt: &updated,
		WebURL:    "https://gitlab.com/owner/project/-/pipelines/456",
	}
}

// FailedPipeline returns a failed GitLab pipeline for testing.
func FailedPipeline(id int, sha string) *gitlab.PipelineInfo {
	now := time.Now()
	updated := now.Add(failedPipelineDuration)
	return &gitlab.PipelineInfo{
		ID:        id,
		SHA:       sha,
		Status:    "failed",
		Ref:       "feature-branch",
		CreatedAt: &now,
		UpdatedAt: &updated,
		WebURL:    "https://gitlab.com/owner/project/-/pipelines/789",
	}
}

// RunningPipeline returns a running GitLab pipeline for testing.
func RunningPipeline(id int, sha string) *gitlab.PipelineInfo {
	now := time.Now()
	return &gitlab.PipelineInfo{
		ID:        id,
		SHA:       sha,
		Status:    "running",
		Ref:       "feature-branch",
		CreatedAt: &now,
		WebURL:    "https://gitlab.com/owner/project/-/pipelines/101",
	}
}

// PendingPipeline returns a pending GitLab pipeline for testing.
func PendingPipeline(id int, sha string) *gitlab.PipelineInfo {
	now := time.Now()
	return &gitlab.PipelineInfo{
		ID:        id,
		SHA:       sha,
		Status:    "pending",
		Ref:       "feature-branch",
		CreatedAt: &now,
		WebURL:    "https://gitlab.com/owner/project/-/pipelines/202",
	}
}

// SuccessfulJob returns a successful GitLab job for testing.
func SuccessfulJob(id int, name, stage string) *glpkg.Job {
	now := time.Now()
	started := now.Add(1 * time.Minute)
	finished := started.Add(successfulJobDuration)
	return &glpkg.Job{
		ID:         id,
		Name:       name,
		Status:     "success",
		Stage:      stage,
		CreatedAt:  now,
		StartedAt:  &started,
		FinishedAt: &finished,
		Duration:   successfulJobDurationSeconds,
		WebURL:     "https://gitlab.com/owner/project/-/jobs/123",
	}
}

// FailedJob returns a failed GitLab job for testing.
func FailedJob(id int, name, stage string) *glpkg.Job {
	now := time.Now()
	started := now.Add(1 * time.Minute)
	finished := started.Add(failedJobDuration)
	return &glpkg.Job{
		ID:         id,
		Name:       name,
		Status:     "failed",
		Stage:      stage,
		CreatedAt:  now,
		StartedAt:  &started,
		FinishedAt: &finished,
		Duration:   failedJobDurationSeconds,
		WebURL:     "https://gitlab.com/owner/project/-/jobs/456",
	}
}

// RunningJob returns a running GitLab job for testing.
func RunningJob(id int, name, stage string) *glpkg.Job {
	now := time.Now()
	started := now.Add(1 * time.Minute)
	return &glpkg.Job{
		ID:        id,
		Name:      name,
		Status:    "running",
		Stage:     stage,
		CreatedAt: now,
		StartedAt: &started,
		WebURL:    "https://gitlab.com/owner/project/-/jobs/789",
	}
}

// PendingJob returns a pending GitLab job for testing.
func PendingJob(id int, name, stage string) *glpkg.Job {
	now := time.Now()
	return &glpkg.Job{
		ID:        id,
		Name:      name,
		Status:    "pending",
		Stage:     stage,
		CreatedAt: now,
		WebURL:    "https://gitlab.com/owner/project/-/jobs/101",
	}
}

// SkippedJob returns a skipped GitLab job for testing.
func SkippedJob(id int, name, stage string) *glpkg.Job {
	now := time.Now()
	return &glpkg.Job{
		ID:        id,
		Name:      name,
		Status:    "skipped",
		Stage:     stage,
		CreatedAt: now,
		WebURL:    "https://gitlab.com/owner/project/-/jobs/202",
	}
}

// ValidGitLabLabels returns a list of valid GitLab labels for testing.
func ValidGitLabLabels() []*glpkg.Label {
	return []*glpkg.Label{
		{Name: "bug"},
		{Name: "enhancement"},
		{Name: "documentation"},
		{Name: "help wanted"},
	}
}

// BasicMergeRequest returns a GitLab basic merge request for testing.
func BasicMergeRequest(iid int, sourceBranch, targetBranch string) *gitlab.BasicMergeRequest {
	return &gitlab.BasicMergeRequest{
		IID:          iid,
		Title:        "Test MR " + sourceBranch,
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		State:        "opened",
		WebURL:       "https://gitlab.com/owner/project/-/merge_requests/" + string(rune(iid)),
	}
}

// MultipleJobs returns a list of jobs in different states for testing pipelines.
func MultipleJobs() []*glpkg.Job {
	return []*glpkg.Job{
		SuccessfulJob(1, "build", "build"),
		SuccessfulJob(jobID2, "test:unit", "test"),
		RunningJob(jobID3, "test:integration", "test"),
		PendingJob(jobID4, "deploy", "deploy"),
	}
}

// CompletedPipelineJobs returns a list of completed jobs for a successful pipeline.
func CompletedPipelineJobs() []*glpkg.Job {
	return []*glpkg.Job{
		SuccessfulJob(1, "build", "build"),
		SuccessfulJob(jobID2, "test:unit", "test"),
		SuccessfulJob(jobID3, "test:integration", "test"),
		SuccessfulJob(jobID4, "deploy", "deploy"),
	}
}

// FailedPipelineJobs returns a list of jobs with one failure.
func FailedPipelineJobs() []*glpkg.Job {
	return []*glpkg.Job{
		SuccessfulJob(1, "build", "build"),
		FailedJob(jobID2, "test:unit", "test"),
		SkippedJob(jobID3, "test:integration", "test"),
		SkippedJob(jobID4, "deploy", "deploy"),
	}
}
