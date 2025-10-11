// Package gitlab provides GitLab API client operations.
package gitlab

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/bullets"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

var (
	errTokenRequired     = errors.New("GITLAB_TOKEN environment variable is required")
	errInvalidURLFormat  = errors.New("invalid GitLab URL format")
	errAssigneeNotFound  = errors.New("failed to find assignee user")
	errReviewerNotFound  = errors.New("failed to find reviewer user")
	errPipelineTimeout   = errors.New("timeout waiting for pipeline completion")
	errMRNotFound        = errors.New("no merge request found for branch")
)

const (
	minURLParts         = 2
	pipelinePollInterval = 5 * time.Second
)

// Client represents a GitLab API client wrapper.
type Client struct {
	client      *gitlab.Client
	projectID   string
	mrIID       int
	mrSHA       string
	log         *bullets.Logger
	updatableLog *bullets.UpdatableLogger
}

// Label represents a GitLab label.
type Label struct {
	Name string
}

// NewClient creates a new GitLab client.
func NewClient() (*Client, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return nil, errTokenRequired
	}

	client, err := gitlab.NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	return &Client{
		client:      client,
		log:         logger.NoLogger(),
		updatableLog: bullets.NewUpdatable(os.Stdout),
	}, nil
}

// SetLogger sets the logger for the GitLab client.
func (c *Client) SetLogger(logger *bullets.Logger) {
	c.log = logger
	c.updatableLog.Logger = logger
	c.log.Debug("GitLab client logger configured")
}

// SetProjectFromURL sets the project from a git remote URL.
func (c *Client) SetProjectFromURL(url string) error {
	// Extract project path from URL
	// Supports both HTTPS and SSH formats:
	// - https://gitlab.com/user/project.git
	// - git@gitlab.com:user/project.git
	url = strings.TrimSuffix(url, ".git")

	projectPath := extractProjectPath(url)
	if projectPath == "" {
		return errInvalidURLFormat
	}

	c.log.Debug("Setting GitLab project: " + projectPath)
	// Get project info to validate and get project ID
	project, _, err := c.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return fmt.Errorf("failed to get project information: %w", err)
	}

	c.projectID = strconv.Itoa(project.ID)
	c.log.Debug("GitLab project set, ID: " + c.projectID)
	return nil
}

// extractProjectPath extracts the project path from a git URL.
func extractProjectPath(url string) string {
	if strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://git@") {
		// SSH format: git@gitlab.com:user/project or ssh://git@gitlab.com/user/project
		parts := strings.Split(url, ":")
		if len(parts) >= minURLParts {
			return parts[len(parts)-1]
		}
		// Handle ssh:// format
		parts = strings.Split(url, "/")
		if len(parts) >= minURLParts {
			return strings.Join(parts[len(parts)-minURLParts:], "/")
		}
	} else {
		// HTTPS format
		parts := strings.Split(url, "/")
		if len(parts) >= minURLParts {
			return strings.Join(parts[len(parts)-minURLParts:], "/")
		}
	}
	return ""
}

// ListLabels returns all labels for the project.
func (c *Client) ListLabels() ([]*Label, error) {
	c.log.Debug("Listing GitLab labels")
	labels, _, err := c.client.Labels.ListLabels(c.projectID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	result := make([]*Label, len(labels))
	for i, label := range labels {
		result[i] = &Label{Name: label.Name}
	}

	c.log.Debug(fmt.Sprintf("Labels retrieved, count: %d", len(labels)))
	return result, nil
}

// CreateMergeRequest creates a new merge request with assignees, reviewers, and labels.
func (c *Client) CreateMergeRequest(
	sourceBranch, targetBranch, title, description, assignee, reviewer string,
	labels []string,
) (*gitlab.MergeRequest, error) {
	c.log.Debug(fmt.Sprintf("Creating merge request from %s to %s", sourceBranch, targetBranch))

	// Get user IDs for assignee and reviewer
	assigneeUser, _, err := c.client.Users.ListUsers(&gitlab.ListUsersOptions{
		Username: &assignee,
	})
	if err != nil || len(assigneeUser) == 0 {
		return nil, fmt.Errorf("%w: %s", errAssigneeNotFound, assignee)
	}

	reviewerUser, _, err := c.client.Users.ListUsers(&gitlab.ListUsersOptions{
		Username: &reviewer,
	})
	if err != nil || len(reviewerUser) == 0 {
		return nil, fmt.Errorf("%w: %s", errReviewerNotFound, reviewer)
	}

	assigneeID := assigneeUser[0].ID
	reviewerIDs := []int{reviewerUser[0].ID}

	labelOptions := (*gitlab.LabelOptions)(&labels)
	createOptions := &gitlab.CreateMergeRequestOptions{
		Title:              &title,
		Description:        &description,
		SourceBranch:       &sourceBranch,
		TargetBranch:       &targetBranch,
		AssigneeID:         &assigneeID,
		ReviewerIDs:        &reviewerIDs,
		Labels:             labelOptions,
		Squash:             gitlab.Ptr(true),
		RemoveSourceBranch: gitlab.Ptr(true),
	}

	mr, _, err := c.client.MergeRequests.CreateMergeRequest(c.projectID, createOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create merge request: %w", err)
	}

	c.mrIID = mr.IID
	c.mrSHA = mr.SHA
	c.log.Debug(fmt.Sprintf("Merge request created - IID: %d, SHA: %s, URL: %s", mr.IID, mr.SHA, mr.WebURL))
	return mr, nil
}

// GetMergeRequestByBranch fetches an existing merge request by source and target branches.
func (c *Client) GetMergeRequestByBranch(sourceBranch, targetBranch string) (*gitlab.MergeRequest, error) {
	mrs, _, err := c.client.MergeRequests.ListProjectMergeRequests(c.projectID, &gitlab.ListProjectMergeRequestsOptions{
		State:        gitlab.Ptr("opened"),
		SourceBranch: &sourceBranch,
		TargetBranch: &targetBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list merge requests: %w", err)
	}

	if len(mrs) == 0 {
		return nil, fmt.Errorf("%w: %s", errMRNotFound, sourceBranch)
	}

	// Get full MR details
	mr, _, err := c.client.MergeRequests.GetMergeRequest(c.projectID, mrs[0].IID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request details: %w", err)
	}

	c.mrIID = mr.IID
	c.mrSHA = mr.SHA
	return mr, nil
}

// WaitForPipeline waits for all pipelines to complete for the merge request.
func (c *Client) WaitForPipeline(timeout time.Duration) (string, error) {
	c.log.Debug(fmt.Sprintf("Waiting for pipeline, timeout: %v", timeout))
	start := time.Now()

	// First check if any pipelines are expected for this commit
	if !c.hasPipelineRuns() {
		c.log.Info("No pipeline runs configured for this merge request, proceeding without checks")
		return "success", nil
	}

	// Create updatable handle for pipeline status
	handle := c.updatableLog.InfoHandle("Waiting for pipeline...")

	for time.Since(start) < timeout {
		pipelines, _, err := c.client.MergeRequests.ListMergeRequestPipelines(c.projectID, c.mrIID, nil)
		if err != nil {
			handle.Error(fmt.Sprintf("Failed to list MR pipelines: %v", err))
			return "", fmt.Errorf("failed to list MR pipelines: %w", err)
		}

		if len(pipelines) == 0 {
			elapsed := time.Since(start)
			handle.Update(bullets.InfoLevel, fmt.Sprintf("Waiting for CI to start... (%s)", formatDuration(elapsed)))
			time.Sleep(pipelinePollInterval)
			continue
		}

		pipeline := pipelines[0]
		status := pipeline.Status

		if status == "running" || status == "pending" || status == "created" {
			elapsed := time.Since(start)
			msg := fmt.Sprintf("Pipeline status: %s - running for %s",
				status, formatDuration(elapsed))
			handle.Update(bullets.InfoLevel, msg)
			time.Sleep(pipelinePollInterval)
			continue
		}

		totalDuration := time.Since(start)
		if status == "success" {
			handle.Success("Pipeline completed successfully - total time: " + formatDuration(totalDuration))
		} else {
			msg := fmt.Sprintf("Pipeline completed with status: %s - total time: %s",
				status, formatDuration(totalDuration))
			handle.Warning(msg)
		}
		return status, nil
	}

	totalDuration := time.Since(start)
	handle.Error("Pipeline timeout after " + formatDuration(totalDuration))
	return "", errPipelineTimeout
}

// ApproveMergeRequest approves a merge request.
func (c *Client) ApproveMergeRequest(mrIID int) error {
	c.log.Debug(fmt.Sprintf("Approving merge request, IID: %d", mrIID))
	_, _, err := c.client.MergeRequestApprovals.ApproveMergeRequest(c.projectID, mrIID, nil)
	if err != nil {
		return fmt.Errorf("failed to approve merge request: %w", err)
	}
	c.log.Debug("Merge request approved")
	return nil
}

// MergeMergeRequest merges a merge request.
func (c *Client) MergeMergeRequest(mrIID int) error {
	c.log.Debug(fmt.Sprintf("Merging merge request, IID: %d", mrIID))
	mergeOptions := &gitlab.AcceptMergeRequestOptions{
		Squash:             gitlab.Ptr(true),
		ShouldRemoveSourceBranch: gitlab.Ptr(true),
	}

	_, _, err := c.client.MergeRequests.AcceptMergeRequest(c.projectID, mrIID, mergeOptions)
	if err != nil {
		return fmt.Errorf("failed to merge MR: %w", err)
	}

	c.log.Debug("Merge request merged successfully")
	return nil
}

// GetMergeRequestsByBranch returns all open merge requests for the given source branch.
func (c *Client) GetMergeRequestsByBranch(sourceBranch string) ([]*gitlab.BasicMergeRequest, error) {
	mrs, _, err := c.client.MergeRequests.ListProjectMergeRequests(c.projectID, &gitlab.ListProjectMergeRequestsOptions{
		SourceBranch: &sourceBranch,
		State:        gitlab.Ptr("opened"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list merge requests: %w", err)
	}

	return mrs, nil
}

// hasPipelineRuns checks if there are any pipeline runs (in any state) for this MR.
func (c *Client) hasPipelineRuns() bool {
	// Check for pipelines associated with this commit SHA
	pipelines, _, err := c.client.Pipelines.ListProjectPipelines(
		c.projectID,
		&gitlab.ListProjectPipelinesOptions{
			SHA: gitlab.Ptr(c.mrSHA),
		},
	)
	if err != nil {
		c.log.Debug(fmt.Sprintf("Failed to list project pipelines, assuming pipelines exist - error: %v", err))
		return true // Assume pipelines exist on error to be safe
	}

	if len(pipelines) > 0 {
		c.log.Debug(fmt.Sprintf("Found pipeline runs for MR, count: %d", len(pipelines)))
		return true
	}

	return false
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	minutes := d / time.Minute
	seconds := (d % time.Minute) / time.Second

	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}