// Package github provides GitHub API client operations.
package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

var (
	errTokenRequired      = errors.New("GITHUB_TOKEN environment variable is required")
	errInvalidURLFormat   = errors.New("invalid GitHub URL format")
	errWorkflowTimeout    = errors.New("timeout waiting for workflow completion")
)

const (
	minURLParts          = 2
	maxCheckRunsPerPage  = 100
	checkPollInterval    = 5 * time.Second
)

// Client represents a GitHub API client wrapper.
type Client struct {
	client   *github.Client
	owner    string
	repo     string
	prNumber int
	prSHA    string
}

// Label represents a GitHub label.
type Label struct {
	Name string
}

// NewClient creates a new GitHub client.
func NewClient() (*Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, errTokenRequired
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &Client{
		client: client,
	}, nil
}

// SetRepositoryFromURL sets the repository from a git remote URL.
func (c *Client) SetRepositoryFromURL(url string) error {
	// Extract owner/repo from URL
	// Supports both HTTPS and SSH formats:
	// - https://github.com/owner/repo.git
	// - git@github.com:owner/repo.git
	url = strings.TrimSuffix(url, ".git")

	var ownerRepo string
	if strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://git@") {
		// SSH format: git@github.com:owner/repo or ssh://git@github.com/owner/repo
		parts := strings.Split(url, ":")
		if len(parts) >= minURLParts {
			ownerRepo = parts[len(parts)-1]
		} else {
			// Handle ssh:// format
			parts = strings.Split(url, "/")
			if len(parts) >= minURLParts {
				ownerRepo = strings.Join(parts[len(parts)-2:], "/")
			}
		}
	} else {
		// HTTPS format
		parts := strings.Split(url, "/")
		if len(parts) >= minURLParts {
			ownerRepo = strings.Join(parts[len(parts)-2:], "/")
		}
	}

	if ownerRepo == "" {
		return errInvalidURLFormat
	}

	parts := strings.Split(ownerRepo, "/")
	if len(parts) != minURLParts {
		return errInvalidURLFormat
	}

	c.owner = parts[0]
	c.repo = parts[1]

	// Validate repository exists
	_, _, err := c.client.Repositories.Get(c.ctx(), c.owner, c.repo)
	if err != nil {
		return fmt.Errorf("failed to get repository information: %w", err)
	}

	return nil
}

// ListLabels returns all labels for the repository.
func (c *Client) ListLabels() ([]*Label, error) {
	labels, _, err := c.client.Issues.ListLabels(c.ctx(), c.owner, c.repo, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	result := make([]*Label, len(labels))
	for i, label := range labels {
		result[i] = &Label{Name: *label.Name}
	}

	return result, nil
}

// CreatePullRequest creates a new pull request with assignees, reviewers, and labels.
func (c *Client) CreatePullRequest(
	head, base, title, body string,
	assignees, reviewers, labels []string,
) (*github.PullRequest, error) {
	newPR := &github.NewPullRequest{
		Title: github.Ptr(title),
		Head:  github.Ptr(head),
		Base:  github.Ptr(base),
		Body:  github.Ptr(body),
	}

	pr, _, err := c.client.PullRequests.Create(c.ctx(), c.owner, c.repo, newPR)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	// Add assignees if provided
	if len(assignees) > 0 {
		_, _, err = c.client.Issues.AddAssignees(c.ctx(), c.owner, c.repo, *pr.Number, assignees)
		if err != nil {
			return nil, fmt.Errorf("failed to add assignees: %w", err)
		}
	}

	// Add reviewers if provided
	if len(reviewers) > 0 {
		reviewRequest := github.ReviewersRequest{
			Reviewers: reviewers,
		}
		_, _, err = c.client.PullRequests.RequestReviewers(c.ctx(), c.owner, c.repo, *pr.Number, reviewRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to add reviewers: %w", err)
		}
	}

	// Add labels if provided
	if len(labels) > 0 {
		_, _, err = c.client.Issues.AddLabelsToIssue(c.ctx(), c.owner, c.repo, *pr.Number, labels)
		if err != nil {
			return nil, fmt.Errorf("failed to add labels: %w", err)
		}
	}

	c.prNumber = *pr.Number
	c.prSHA = *pr.Head.SHA
	return pr, nil
}

// WaitForWorkflows waits for all workflow runs to complete for the pull request.
func (c *Client) WaitForWorkflows(timeout time.Duration) (string, error) {
	start := time.Now()

	for time.Since(start) < timeout {
		checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(
			c.ctx(), c.owner, c.repo, c.prSHA,
			&github.ListCheckRunsOptions{
				ListOptions: github.ListOptions{PerPage: maxCheckRunsPerPage},
			},
		)
		if err != nil {
			return "", fmt.Errorf("failed to list check runs: %w", err)
		}

		if checkRuns.GetTotal() == 0 {
			time.Sleep(checkPollInterval)
			continue
		}

		allCompleted, conclusion := c.processCheckRuns(checkRuns.CheckRuns)
		if !allCompleted {
			time.Sleep(checkPollInterval)
			continue
		}

		return conclusion, nil
	}

	return "", errWorkflowTimeout
}

// MergePullRequest merges a pull request using the specified merge method.
func (c *Client) MergePullRequest(prNumber int, mergeMethod string) error {
	options := &github.PullRequestOptions{
		MergeMethod: mergeMethod, // "squash", "merge", or "rebase"
	}

	_, _, err := c.client.PullRequests.Merge(c.ctx(), c.owner, c.repo, prNumber, "", options)
	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	return nil
}

// GetPullRequestsByHead returns all open pull requests for the given head branch.
func (c *Client) GetPullRequestsByHead(head string) ([]*github.PullRequest, error) {
	prs, _, err := c.client.PullRequests.List(c.ctx(), c.owner, c.repo, &github.PullRequestListOptions{
		Head:  fmt.Sprintf("%s:%s", c.owner, head),
		State: "open",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	return prs, nil
}

// DeleteBranch deletes a branch from the remote repository.
func (c *Client) DeleteBranch(branch string) error {
	_, err := c.client.Git.DeleteRef(c.ctx(), c.owner, c.repo, "heads/"+branch)
	if err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}
	return nil
}

// ctx returns the context for API calls.
func (c *Client) ctx() context.Context {
	return context.Background()
}

// processCheckRuns evaluates check run statuses and returns completion state and overall conclusion.
func (c *Client) processCheckRuns(checks []*github.CheckRun) (bool, string) {
	allCompleted := true
	conclusion := "success"

	for _, check := range checks {
		status := check.GetStatus()
		if status == "in_progress" || status == "queued" {
			allCompleted = false
			fmt.Printf("Check '%s' is still %s...\n", check.GetName(), status)
			break
		}

		checkConclusion := check.GetConclusion()
		if checkConclusion != "success" && checkConclusion != "skipped" && checkConclusion != "neutral" {
			conclusion = checkConclusion
		}
	}

	return allCompleted, conclusion
}