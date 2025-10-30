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
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/bullets"
	"golang.org/x/oauth2"
)

var (
	errTokenRequired      = errors.New("GITHUB_TOKEN environment variable is required")
	errInvalidURLFormat   = errors.New("invalid GitHub URL format")
	errWorkflowTimeout    = errors.New("timeout waiting for workflow completion")
	errPRNotFound         = errors.New("no pull request found for branch")
)

const (
	minURLParts          = 2
	maxCheckRunsPerPage  = 100
	checkPollInterval    = 5 * time.Second
	conclusionSuccess    = "success"
	statusInProgress     = "in_progress"
	statusQueued         = "queued"
	statusCompleted      = "completed"
	conclusionSkipped    = "skipped"
	conclusionNeutral    = "neutral"
)

// Client represents a GitHub API client wrapper.
type Client struct {
	client      *github.Client
	owner       string
	repo        string
	prNumber    int
	prSHA       string
	log         *bullets.Logger
	updatableLog *bullets.UpdatableLogger
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
		client:      client,
		log:         logger.NoLogger(),
		updatableLog: bullets.NewUpdatable(os.Stdout),
	}, nil
}

// SetLogger sets the logger for the GitHub client.
func (c *Client) SetLogger(logger *bullets.Logger) {
	c.log = logger
	c.updatableLog.Logger = logger
	c.log.Debug("GitHub client logger configured")
}

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

// WaitForWorkflows waits for all workflow runs to complete for the pull request.
func (c *Client) WaitForWorkflows(timeout time.Duration) (string, error) {
	c.log.Debug(fmt.Sprintf("Waiting for workflows, timeout: %v", timeout))
	start := time.Now()

	// First check if any workflow runs are expected for this PR
	if !c.hasWorkflowRuns() {
		c.log.Info("No workflow runs configured for this pull request, proceeding without checks")
		return conclusionSuccess, nil
	}

	// Create updatable handle for workflow status
	c.updatableLog.Info("Waiting for workflows to complete...")
	c.updatableLog.IncreasePadding()
	defer c.updatableLog.DecreasePadding()

	handle := c.updatableLog.InfoHandle("Checking status...")

	for time.Since(start) < timeout {
		checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(
			c.ctx(), c.owner, c.repo, c.prSHA,
			&github.ListCheckRunsOptions{
				ListOptions: github.ListOptions{PerPage: maxCheckRunsPerPage},
			},
		)
		if err != nil {
			handle.Error(fmt.Sprintf("Failed to list check runs: %v", err))
			return "", fmt.Errorf("failed to list check runs: %w", err)
		}

		if checkRuns.GetTotal() == 0 {
			elapsed := time.Since(start)
			handle.Update(bullets.InfoLevel, fmt.Sprintf("Waiting for workflows to start... (%s)", formatDuration(elapsed)))
			time.Sleep(checkPollInterval)
			continue
		}

		allCompleted, conclusion := c.processCheckRuns(checkRuns.CheckRuns, handle, start)
		if !allCompleted {
			time.Sleep(checkPollInterval)
			continue
		}

		totalDuration := time.Since(start)
		if conclusion == conclusionSuccess {
			handle.Success("All checks passed - total time: " + formatDuration(totalDuration))
		} else {
			msg := fmt.Sprintf("Checks completed with status: %s - total time: %s",
				conclusion, formatDuration(totalDuration))
			handle.Warning(msg)
		}
		return conclusion, nil
	}

	totalDuration := time.Since(start)
	handle.Error("Timeout after " + formatDuration(totalDuration))
	return "", errWorkflowTimeout
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

// ctx returns the context for API calls.
func (c *Client) ctx() context.Context {
	return context.Background()
}

// GetMergeMethod returns the appropriate merge method string based on squash flag.
// Returns "squash" if squash is true, otherwise "merge".
func GetMergeMethod(squash bool) string {
	if squash {
		return "squash"
	}
	return "merge"
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

// checkStats holds counters for check run statuses.
type checkStats struct {
	running        int
	queued         int
	succeeded      int
	failed         int
	skipped        int
	runningChecks  []string
}

// processCheckRuns evaluates check run statuses and returns completion state and overall conclusion.
func (c *Client) processCheckRuns(
	checks []*github.CheckRun, handle *bullets.BulletHandle, startTime time.Time,
) (bool, string) {
	allCompleted := true
	conclusion := conclusionSuccess
	elapsed := time.Since(startTime)

	stats := c.collectCheckStats(checks, &allCompleted, &conclusion)
	statusMsg := c.buildCheckStatusMessage(stats, len(checks), elapsed)

	handle.Update(bullets.InfoLevel, statusMsg)
	return allCompleted, conclusion
}

// collectCheckStats collects statistics from all check runs.
func (c *Client) collectCheckStats(
	checks []*github.CheckRun, allCompleted *bool, conclusion *string,
) checkStats {
	stats := checkStats{}

	for _, check := range checks {
		status := check.GetStatus()

		switch status {
		case statusInProgress:
			*allCompleted = false
			stats.running++
			stats.runningChecks = append(stats.runningChecks, check.GetName())
		case statusQueued:
			*allCompleted = false
			stats.queued++
		case statusCompleted:
			c.processCompletedCheck(check, &stats, conclusion)
		}
	}

	return stats
}

// processCompletedCheck processes a completed check and updates stats.
func (c *Client) processCompletedCheck(
	check *github.CheckRun, stats *checkStats, conclusion *string,
) {
	checkConclusion := check.GetConclusion()

	switch checkConclusion {
	case conclusionSuccess:
		stats.succeeded++
	case conclusionSkipped, conclusionNeutral:
		stats.skipped++
	default:
		stats.failed++
		// Update overall conclusion if this check failed
		if *conclusion == conclusionSuccess {
			*conclusion = checkConclusion
		}
	}
}

// buildCheckStatusMessage builds a status message from check statistics.
func (c *Client) buildCheckStatusMessage(stats checkStats, total int, elapsed time.Duration) string {
	var statusParts []string

	if stats.running > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d running", stats.running))
	}
	if stats.queued > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d queued", stats.queued))
	}
	if stats.succeeded > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d passed", stats.succeeded))
	}
	if stats.failed > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d failed", stats.failed))
	}
	if stats.skipped > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d skipped", stats.skipped))
	}

	statusMsg := fmt.Sprintf("Checks: %s (total: %d) - %s",
		strings.Join(statusParts, ", "), total, formatDuration(elapsed))

	// Add currently running check names for context
	if len(stats.runningChecks) > 0 && len(stats.runningChecks) <= 3 {
		statusMsg += fmt.Sprintf(" [%s]", strings.Join(stats.runningChecks, ", "))
	}

	return statusMsg
}