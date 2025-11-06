// Package github provides GitHub API client operations.
package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v69/github"
)

// SetRepositoryFromURL sets the repository from a git remote URL.
func (c *Client) SetRepositoryFromURL(url string) error {
	// Extract owner/repo from URL
	// Supports both HTTPS and SSH formats:
	// - https://github.com/owner/repo.git
	// - git@github.com:owner/repo.git
	url = strings.TrimSuffix(url, ".git")

	ownerRepo := extractOwnerRepo(url)
	if ownerRepo == "" {
		return errInvalidURLFormat
	}

	parts := strings.Split(ownerRepo, "/")
	if len(parts) != minURLParts {
		return errInvalidURLFormat
	}

	c.owner = parts[0]
	c.repo = parts[1]

	c.log.Debug(fmt.Sprintf("Setting GitHub repository: %s/%s", c.owner, c.repo))
	// Validate repository exists
	_, _, err := c.client.Repositories.Get(c.ctx(), c.owner, c.repo)
	if err != nil {
		return fmt.Errorf("failed to get repository information: %w", err)
	}

	c.log.Debug("GitHub repository set successfully")
	return nil
}

// extractOwnerRepo extracts the owner/repo path from a git URL.
func extractOwnerRepo(url string) string {
	if strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://git@") {
		// SSH format: git@github.com:owner/repo or ssh://git@github.com/owner/repo
		parts := strings.Split(url, ":")
		if len(parts) >= minURLParts {
			return parts[len(parts)-1]
		}
		// Handle ssh:// format
		parts = strings.Split(url, "/")
		if len(parts) >= minURLParts {
			return strings.Join(parts[len(parts)-2:], "/")
		}
	} else {
		// HTTPS format
		parts := strings.Split(url, "/")
		if len(parts) >= minURLParts {
			return strings.Join(parts[len(parts)-2:], "/")
		}
	}
	return ""
}

// ListLabels returns all labels for the repository.
func (c *Client) ListLabels() ([]*Label, error) {
	c.log.Debug("Listing GitHub labels")
	labels, _, err := c.client.Issues.ListLabels(c.ctx(), c.owner, c.repo, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	result := make([]*Label, len(labels))
	for i, label := range labels {
		result[i] = &Label{Name: *label.Name}
	}

	c.log.Debug(fmt.Sprintf("Labels retrieved, count: %d", len(labels)))
	return result, nil
}

// CreatePullRequest creates a new pull request with assignees, reviewers, and labels.
func (c *Client) CreatePullRequest(
	head, base, title, body string,
	assignees, reviewers, labels []string,
) (*github.PullRequest, error) {
	c.log.Debug(fmt.Sprintf("Creating pull request from %s to %s", head, base))

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

	// Add reviewers if provided (filter out PR author)
	if len(reviewers) > 0 {
		if err := c.addReviewers(pr, reviewers); err != nil {
			return nil, err
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
	c.log.Debug(fmt.Sprintf("Pull request created - number: %d, URL: %s", c.prNumber, *pr.HTMLURL))
	return pr, nil
}

// GetPullRequestByBranch fetches an existing pull request by head and base branches.
func (c *Client) GetPullRequestByBranch(head, base string) (*github.PullRequest, error) {
	prs, _, err := c.client.PullRequests.List(c.ctx(), c.owner, c.repo, &github.PullRequestListOptions{
		State: "open",
		Head:  fmt.Sprintf("%s:%s", c.owner, head),
		Base:  base,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	if len(prs) == 0 {
		return nil, fmt.Errorf("%w: %s", errPRNotFound, head)
	}

	pr := prs[0]
	c.prNumber = *pr.Number
	c.prSHA = *pr.Head.SHA
	return pr, nil
}

// MergePullRequest merges a pull request using the specified merge method.
// mergeMethod can be "merge", "squash", or "rebase".
func (c *Client) MergePullRequest(prNumber int, mergeMethod string) error {
	c.log.Debug(fmt.Sprintf("Merging pull request #%d using method: %s", prNumber, mergeMethod))
	options := &github.PullRequestOptions{
		MergeMethod: mergeMethod, // "squash", "merge", or "rebase"
	}

	_, _, err := c.client.PullRequests.Merge(c.ctx(), c.owner, c.repo, prNumber, "", options)
	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	c.log.Debug("Pull request merged successfully")
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

// addReviewers adds reviewers to a pull request, filtering out the PR author.
func (c *Client) addReviewers(pr *github.PullRequest, reviewers []string) error {
	prAuthor := pr.User.GetLogin()
	filteredReviewers := make([]string, 0, len(reviewers))
	for _, reviewer := range reviewers {
		if reviewer != prAuthor {
			filteredReviewers = append(filteredReviewers, reviewer)
		}
	}

	if len(filteredReviewers) > 0 {
		reviewRequest := github.ReviewersRequest{
			Reviewers: filteredReviewers,
		}
		_, _, err := c.client.PullRequests.RequestReviewers(c.ctx(), c.owner, c.repo, *pr.Number, reviewRequest)
		if err != nil {
			return fmt.Errorf("failed to add reviewers: %w", err)
		}
	}
	return nil
}

// hasWorkflowRuns checks if there are any workflow runs (in any state) for this PR.
func (c *Client) hasWorkflowRuns() bool {
	// Check for workflow runs associated with this commit SHA
	runs, _, err := c.client.Actions.ListRepositoryWorkflowRuns(
		c.ctx(), c.owner, c.repo,
		&github.ListWorkflowRunsOptions{
			Event:   "pull_request",
			HeadSHA: c.prSHA,
		},
	)
	if err != nil {
		c.log.Debug(fmt.Sprintf("Failed to list workflow runs, assuming workflows exist - error: %v", err))
		return true // Assume workflows exist on error to be safe
	}

	if runs.GetTotalCount() > 0 {
		c.log.Debug(fmt.Sprintf("Found workflow runs for PR, count: %d", runs.GetTotalCount()))
		return true
	}

	// Also check suites as they're created even before runs start
	checkSuites, _, err := c.client.Checks.ListCheckSuitesForRef(
		c.ctx(), c.owner, c.repo, c.prSHA,
		&github.ListCheckSuiteOptions{},
	)
	if err != nil {
		c.log.Debug(fmt.Sprintf("Failed to list check suites, assuming workflows exist - error: %v", err))
		return true // Assume workflows exist on error to be safe
	}

	if checkSuites.GetTotal() > 0 {
		c.log.Debug(fmt.Sprintf("Found check suites for PR, count: %d", checkSuites.GetTotal()))
		return true
	}

	return false
}

// fetchWorkflowJobs fetches all jobs for workflow runs associated with the PR SHA.
func (c *Client) fetchWorkflowJobs() ([]*JobInfo, error) {
	c.log.Debug("Fetching workflow jobs for PR")

	// First, get workflow runs for this PR
	runs, _, err := c.client.Actions.ListRepositoryWorkflowRuns(
		c.ctx(), c.owner, c.repo,
		&github.ListWorkflowRunsOptions{
			Event:   "pull_request",
			HeadSHA: c.prSHA,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}

	if runs.GetTotalCount() == 0 {
		c.log.Debug("No workflow runs found for PR")
		return nil, nil
	}

	// Collect all jobs from all workflow runs
	var allJobs []*JobInfo
	for _, run := range runs.WorkflowRuns {
		jobs, err := c.fetchJobsForRun(run.GetID())
		if err != nil {
			return nil, err
		}
		allJobs = append(allJobs, jobs...)
	}

	c.log.Debug(fmt.Sprintf("Fetched %d total jobs across all workflow runs", len(allJobs)))
	return allJobs, nil
}

// fetchJobsForRun fetches all jobs for a specific workflow run with pagination.
func (c *Client) fetchJobsForRun(runID int64) ([]*JobInfo, error) {
	var allJobs []*JobInfo
	page := 1
	perPage := 100

	for {
		jobs, resp, err := c.client.Actions.ListWorkflowJobs(
			c.ctx(), c.owner, c.repo, runID,
			&github.ListWorkflowJobsOptions{
				ListOptions: github.ListOptions{
					Page:    page,
					PerPage: perPage,
				},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list workflow jobs for run %d: %w", runID, err)
		}

		// Convert GitHub workflow jobs to our JobInfo struct
		for _, ghJob := range jobs.Jobs {
			job := &JobInfo{
				ID:          ghJob.GetID(),
				Name:        ghJob.GetName(),
				Status:      ghJob.GetStatus(),
				Conclusion:  ghJob.GetConclusion(),
				StartedAt:   ghJob.StartedAt.GetTime(),
				CompletedAt: ghJob.CompletedAt.GetTime(),
				HTMLURL:     ghJob.GetHTMLURL(),
			}
			allJobs = append(allJobs, job)
		}

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	return allJobs, nil
}

// convertCheckRunsToJobInfo converts GitHub CheckRuns to JobInfo format.
func (c *Client) convertCheckRunsToJobInfo(checkRuns []*github.CheckRun) []*JobInfo {
	jobs := make([]*JobInfo, 0, len(checkRuns))
	for _, check := range checkRuns {
		if check == nil || check.ID == nil {
			continue
		}

		job := &JobInfo{
			ID:         *check.ID,
			Name:       check.GetName(),
			Status:     check.GetStatus(),
			Conclusion: check.GetConclusion(),
			HTMLURL:    check.GetHTMLURL(),
		}

		// Set timestamps if available
		if check.StartedAt != nil {
			job.StartedAt = check.StartedAt.GetTime()
		}
		if check.CompletedAt != nil {
			job.CompletedAt = check.CompletedAt.GetTime()
		}

		jobs = append(jobs, job)
	}
	return jobs
}

// ctx returns the context for API calls.
func (c *Client) ctx() context.Context {
	return context.Background()
}

// Ensure Client implements APIClient interface at compile time.
var _ APIClient = (*Client)(nil)
