// Package gitlab provides GitLab API client operations.
package gitlab

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

var (
	errTokenRequired     = errors.New("GITLAB_TOKEN environment variable is required")
	errInvalidURLFormat  = errors.New("invalid GitLab URL format")
	errAssigneeNotFound  = errors.New("failed to find assignee user")
	errReviewerNotFound  = errors.New("failed to find reviewer user")
	errPipelineTimeout   = errors.New("timeout waiting for pipeline completion")
)

const (
	minURLParts         = 2
	pipelinePollInterval = 5 * time.Second
)

// Client represents a GitLab API client wrapper.
type Client struct {
	client    *gitlab.Client
	projectID string
	mrIID     int
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

	return &Client{client: client}, nil
}

// SetProjectFromURL sets the project from a git remote URL.
func (c *Client) SetProjectFromURL(url string) error {
	// Extract project path from URL
	// e.g., https://gitlab.com/user/project.git -> user/project
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) < minURLParts {
		return errInvalidURLFormat
	}

	projectPath := strings.Join(parts[len(parts)-minURLParts:], "/")

	// Get project info to validate and get project ID
	project, _, err := c.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return fmt.Errorf("failed to get project information: %w", err)
	}

	c.projectID = strconv.Itoa(project.ID)
	return nil
}

// ListLabels returns all labels for the project.
func (c *Client) ListLabels() ([]*Label, error) {
	labels, _, err := c.client.Labels.ListLabels(c.projectID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	result := make([]*Label, len(labels))
	for i, label := range labels {
		result[i] = &Label{Name: label.Name}
	}

	return result, nil
}

// CreateMergeRequest creates a new merge request with assignees, reviewers, and labels.
func (c *Client) CreateMergeRequest(
	sourceBranch, targetBranch, title, description, assignee, reviewer string,
	labels []string,
) (*gitlab.MergeRequest, error) {
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
	return mr, nil
}

// WaitForPipeline waits for all pipelines to complete for the merge request.
func (c *Client) WaitForPipeline(timeout time.Duration) (string, error) {
	start := time.Now()

	for time.Since(start) < timeout {
		pipelines, _, err := c.client.MergeRequests.ListMergeRequestPipelines(c.projectID, c.mrIID, nil)
		if err != nil {
			return "", fmt.Errorf("failed to list MR pipelines: %w", err)
		}

		if len(pipelines) == 0 {
			time.Sleep(pipelinePollInterval)
			continue
		}

		pipeline := pipelines[0]
		status := pipeline.Status

		if status == "running" || status == "pending" || status == "created" {
			fmt.Printf("Pipeline is still %s...\n", status)
			time.Sleep(pipelinePollInterval)
			continue
		}

		return status, nil
	}

	return "", errPipelineTimeout
}

// ApproveMergeRequest approves a merge request.
func (c *Client) ApproveMergeRequest(mrIID int) error {
	_, _, err := c.client.MergeRequestApprovals.ApproveMergeRequest(c.projectID, mrIID, nil)
	if err != nil {
		return fmt.Errorf("failed to approve merge request: %w", err)
	}
	return nil
}

// MergeMergeRequest merges a merge request.
func (c *Client) MergeMergeRequest(mrIID int) error {
	mergeOptions := &gitlab.AcceptMergeRequestOptions{
		Squash:             gitlab.Ptr(true),
		ShouldRemoveSourceBranch: gitlab.Ptr(true),
	}

	_, _, err := c.client.MergeRequests.AcceptMergeRequest(c.projectID, mrIID, mergeOptions)
	if err != nil {
		return fmt.Errorf("failed to merge MR: %w", err)
	}

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