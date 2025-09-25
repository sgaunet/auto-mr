package gitlab

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/xanzy/go-gitlab"
)

type Client struct {
	client    *gitlab.Client
	projectID string
}

type Label struct {
	Name string
}

func NewClient() (*Client, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITLAB_TOKEN environment variable is required")
	}

	client, err := gitlab.NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	return &Client{client: client}, nil
}

func (c *Client) SetProjectFromURL(url string) error {
	// Extract project path from URL
	// e.g., https://gitlab.com/user/project.git -> user/project
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid GitLab URL format")
	}

	projectPath := strings.Join(parts[len(parts)-2:], "/")

	// Get project info to validate and get project ID
	project, _, err := c.client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return fmt.Errorf("failed to get project information: %w", err)
	}

	c.projectID = fmt.Sprintf("%d", project.ID)
	return nil
}

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

func (c *Client) CreateMergeRequest(sourceBranch, targetBranch, title, description string, assignee, reviewer string, labels []string) (*gitlab.MergeRequest, error) {
	// Get user IDs for assignee and reviewer
	assigneeUser, _, err := c.client.Users.ListUsers(&gitlab.ListUsersOptions{
		Username: &assignee,
	})
	if err != nil || len(assigneeUser) == 0 {
		return nil, fmt.Errorf("failed to find assignee user: %s", assignee)
	}

	reviewerUser, _, err := c.client.Users.ListUsers(&gitlab.ListUsersOptions{
		Username: &reviewer,
	})
	if err != nil || len(reviewerUser) == 0 {
		return nil, fmt.Errorf("failed to find reviewer user: %s", reviewer)
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

	return mr, nil
}

func (c *Client) WaitForPipeline(timeout time.Duration) (string, error) {
	start := time.Now()

	for time.Since(start) < timeout {
		pipelines, _, err := c.client.Pipelines.ListProjectPipelines(c.projectID, &gitlab.ListProjectPipelinesOptions{
			ListOptions: gitlab.ListOptions{PerPage: 1},
		})
		if err != nil {
			return "", fmt.Errorf("failed to list pipelines: %w", err)
		}

		if len(pipelines) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		pipeline := pipelines[0]
		status := pipeline.Status

		if status == "running" || status == "pending" || status == "created" {
			fmt.Printf("Pipeline is still %s...\n", status)
			time.Sleep(5 * time.Second)
			continue
		}

		return status, nil
	}

	return "", fmt.Errorf("timeout waiting for pipeline completion")
}

func (c *Client) ApproveMergeRequest(mrIID int) error {
	_, _, err := c.client.MergeRequestApprovals.ApproveMergeRequest(c.projectID, mrIID, nil)
	if err != nil {
		return fmt.Errorf("failed to approve merge request: %w", err)
	}
	return nil
}

func (c *Client) MergeMergeRequest(mrIID int) error {
	mergeOptions := &gitlab.AcceptMergeRequestOptions{
		Squash:             gitlab.Ptr(true),
		ShouldRemoveSourceBranch: gitlab.Ptr(true),
	}

	_, _, err := c.client.MergeRequests.AcceptMergeRequest(c.projectID, mrIID, mergeOptions)
	if err != nil {
		return fmt.Errorf("failed to merge merge request: %w", err)
	}

	return nil
}

func (c *Client) GetMergeRequestsByBranch(sourceBranch string) ([]*gitlab.MergeRequest, error) {
	mrs, _, err := c.client.MergeRequests.ListProjectMergeRequests(c.projectID, &gitlab.ListProjectMergeRequestsOptions{
		SourceBranch: &sourceBranch,
		State:        gitlab.Ptr("opened"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list merge requests: %w", err)
	}

	return mrs, nil
}