package github

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

type Client struct {
	client *github.Client
	ctx    context.Context
	owner  string
	repo   string
}

type Label struct {
	Name string
}

func NewClient() (*Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &Client{
		client: client,
		ctx:    ctx,
	}, nil
}

func (c *Client) SetRepositoryFromURL(url string) error {
	// Extract owner/repo from URL
	// e.g., https://github.com/owner/repo.git -> owner/repo
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid GitHub URL format")
	}

	c.owner = parts[len(parts)-2]
	c.repo = parts[len(parts)-1]

	// Validate repository exists
	_, _, err := c.client.Repositories.Get(c.ctx, c.owner, c.repo)
	if err != nil {
		return fmt.Errorf("failed to get repository information: %w", err)
	}

	return nil
}

func (c *Client) ListLabels() ([]*Label, error) {
	labels, _, err := c.client.Issues.ListLabels(c.ctx, c.owner, c.repo, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	result := make([]*Label, len(labels))
	for i, label := range labels {
		result[i] = &Label{Name: *label.Name}
	}

	return result, nil
}

func (c *Client) CreatePullRequest(head, base, title, body string, assignees, reviewers, labels []string) (*github.PullRequest, error) {
	newPR := &github.NewPullRequest{
		Title: github.Ptr(title),
		Head:  github.Ptr(head),
		Base:  github.Ptr(base),
		Body:  github.Ptr(body),
	}

	pr, _, err := c.client.PullRequests.Create(c.ctx, c.owner, c.repo, newPR)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	// Add assignees if provided
	if len(assignees) > 0 {
		_, _, err = c.client.Issues.AddAssignees(c.ctx, c.owner, c.repo, *pr.Number, assignees)
		if err != nil {
			return nil, fmt.Errorf("failed to add assignees: %w", err)
		}
	}

	// Add reviewers if provided
	if len(reviewers) > 0 {
		reviewRequest := github.ReviewersRequest{
			Reviewers: reviewers,
		}
		_, _, err = c.client.PullRequests.RequestReviewers(c.ctx, c.owner, c.repo, *pr.Number, reviewRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to add reviewers: %w", err)
		}
	}

	// Add labels if provided
	if len(labels) > 0 {
		_, _, err = c.client.Issues.AddLabelsToIssue(c.ctx, c.owner, c.repo, *pr.Number, labels)
		if err != nil {
			return nil, fmt.Errorf("failed to add labels: %w", err)
		}
	}

	return pr, nil
}

func (c *Client) WaitForWorkflows(timeout time.Duration) (string, error) {
	start := time.Now()

	for time.Since(start) < timeout {
		runs, _, err := c.client.Actions.ListRepositoryWorkflowRuns(c.ctx, c.owner, c.repo, &github.ListWorkflowRunsOptions{
			ListOptions: github.ListOptions{PerPage: 1},
		})
		if err != nil {
			return "", fmt.Errorf("failed to list workflow runs: %w", err)
		}

		if len(runs.WorkflowRuns) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		run := runs.WorkflowRuns[0]
		status := *run.Status

		if status == "in_progress" || status == "queued" {
			fmt.Printf("Workflow is still %s...\n", status)
			time.Sleep(5 * time.Second)
			continue
		}

		if run.Conclusion != nil {
			return *run.Conclusion, nil
		}

		return status, nil
	}

	return "", fmt.Errorf("timeout waiting for workflow completion")
}

func (c *Client) MergePullRequest(prNumber int, mergeMethod string) error {
	options := &github.PullRequestOptions{
		MergeMethod: mergeMethod, // "squash", "merge", or "rebase"
	}

	_, _, err := c.client.PullRequests.Merge(c.ctx, c.owner, c.repo, prNumber, "", options)
	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	return nil
}

func (c *Client) GetPullRequestsByHead(head string) ([]*github.PullRequest, error) {
	prs, _, err := c.client.PullRequests.List(c.ctx, c.owner, c.repo, &github.PullRequestListOptions{
		Head:  fmt.Sprintf("%s:%s", c.owner, head),
		State: "open",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	return prs, nil
}

func (c *Client) DeleteBranch(branch string) error {
	_, err := c.client.Git.DeleteRef(c.ctx, c.owner, c.repo, fmt.Sprintf("heads/%s", branch))
	if err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}
	return nil
}