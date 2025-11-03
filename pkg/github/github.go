// Package github provides GitHub API client operations.
package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
	minURLParts            = 2
	maxCheckRunsPerPage    = 100
	maxJobDetailsToDisplay = 3
	checkPollInterval      = 5 * time.Second
	conclusionSuccess      = "success"
	statusInProgress       = "in_progress"
	statusQueued           = "queued"
	statusCompleted        = "completed"
	conclusionSkipped      = "skipped"
	conclusionNeutral      = "neutral"

	// Status icons for visual representation
	iconRunning  = "⟳"
	iconPending  = "○"
	iconSuccess  = "✓"
	iconFailed   = "✗"
	iconCanceled = "⊘"
	iconSkipped  = "○"
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

// JobInfo represents a GitHub workflow job with detailed status information.
type JobInfo struct {
	ID          int64
	Name        string
	Status      string
	Conclusion  string
	StartedAt   *time.Time
	CompletedAt *time.Time
	HTMLURL     string
}

// checkTracker tracks workflow jobs/checks and their display handles with thread-safe access.
type checkTracker struct {
	mu      sync.RWMutex
	checks  map[int64]*JobInfo
	handles map[int64]*bullets.BulletHandle
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

	// Initialize check tracker for managing individual job handles
	tracker := newCheckTracker()

	for time.Since(start) < timeout {
		checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(
			c.ctx(), c.owner, c.repo, c.prSHA,
			&github.ListCheckRunsOptions{
				ListOptions: github.ListOptions{PerPage: maxCheckRunsPerPage},
			},
		)
		if err != nil {
			c.updatableLog.Error(fmt.Sprintf("Failed to list check runs: %v", err))
			return "", fmt.Errorf("failed to list check runs: %w", err)
		}

		if checkRuns.GetTotal() == 0 {
			elapsed := time.Since(start)
			c.updatableLog.Info(fmt.Sprintf("Waiting for workflows to start... (%s)", formatDuration(elapsed)))
			time.Sleep(checkPollInterval)
			continue
		}

		// Try to fetch and display job-level information with check tracker
		allCompleted, conclusion := c.processWorkflowsWithJobTracking(tracker, start)

		if !allCompleted {
			time.Sleep(checkPollInterval)
			continue
		}

		totalDuration := time.Since(start)
		if conclusion == conclusionSuccess {
			c.updatableLog.Success("All checks passed - total time: " + formatDuration(totalDuration))
		} else {
			msg := fmt.Sprintf("Checks completed with status: %s - total time: %s",
				conclusion, formatDuration(totalDuration))
			handle := c.updatableLog.InfoHandle(msg)
			handle.Warning(msg)
		}
		return conclusion, nil
	}

	totalDuration := time.Since(start)
	c.updatableLog.Error("Timeout after " + formatDuration(totalDuration))
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

// processWorkflowsWithJobTracking processes workflows using checkTracker for individual job display.
func (c *Client) processWorkflowsWithJobTracking(tracker *checkTracker, startTime time.Time) (bool, string) {
	allCompleted := true
	conclusion := conclusionSuccess

	// Try to fetch workflow jobs
	jobs, err := c.fetchWorkflowJobs()
	if err != nil {
		c.log.Debug(fmt.Sprintf("Failed to fetch workflow jobs, falling back to check runs: %v", err))
		// Fall back to check runs
		checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(
			c.ctx(), c.owner, c.repo, c.prSHA,
			&github.ListCheckRunsOptions{
				ListOptions: github.ListOptions{PerPage: maxCheckRunsPerPage},
			},
		)
		if err == nil && checkRuns.GetTotal() > 0 {
			return c.processCheckRunsFallback(nil, startTime)
		}
		return false, ""
	}

	// If no jobs found, fall back to check runs
	if len(jobs) == 0 {
		c.log.Debug("No workflow jobs found, falling back to check runs")
		checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(
			c.ctx(), c.owner, c.repo, c.prSHA,
			&github.ListCheckRunsOptions{
				ListOptions: github.ListOptions{PerPage: maxCheckRunsPerPage},
			},
		)
		if err == nil && checkRuns.GetTotal() > 0 {
			return c.processCheckRunsFallback(nil, startTime)
		}
		return false, ""
	}

	// Update check tracker with new jobs (creates/updates handles automatically)
	tracker.update(jobs, c.updatableLog)

	// Analyze job statuses for completion
	for _, job := range jobs {
		switch job.Status {
		case statusInProgress, statusQueued:
			allCompleted = false
		case statusCompleted:
			// Check conclusion for completed jobs
			switch job.Conclusion {
			case conclusionSuccess:
				// Success - no change needed
			case conclusionSkipped, conclusionNeutral:
				// Neutral/skipped - no change needed
			default:
				// Failed, cancelled, or other non-success conclusion
				if conclusion == conclusionSuccess {
					conclusion = job.Conclusion
				}
			}
		}
	}

	return allCompleted, conclusion
}

// processWorkflows tries to fetch and display job-level information, with fallback to check runs.
func (c *Client) processWorkflows(handle *bullets.BulletHandle, startTime time.Time) (bool, string) {
	elapsed := time.Since(startTime)

	// Try to fetch workflow jobs
	jobs, err := c.fetchWorkflowJobs()
	if err != nil {
		c.log.Debug(fmt.Sprintf("Failed to fetch workflow jobs, falling back to check runs: %v", err))
		return c.processCheckRunsFallback(handle, startTime)
	}

	// If no jobs found, fall back to check runs
	if len(jobs) == 0 {
		c.log.Debug("No workflow jobs found, falling back to check runs")
		return c.processCheckRunsFallback(handle, startTime)
	}

	// Display job-level information
	allCompleted := true
	conclusion := conclusionSuccess

	jobsByStatus := c.analyzeJobStatuses(jobs, &allCompleted, &conclusion)
	statusMsg := c.buildJobStatusMessage(jobsByStatus, jobs, elapsed)
	handle.Update(bullets.InfoLevel, statusMsg)

	return allCompleted, conclusion
}

// processCheckRunsFallback fetches check runs and processes them as fallback.
func (c *Client) processCheckRunsFallback(handle *bullets.BulletHandle, startTime time.Time) (bool, string) {
	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(
		c.ctx(), c.owner, c.repo, c.prSHA,
		&github.ListCheckRunsOptions{
			ListOptions: github.ListOptions{PerPage: maxCheckRunsPerPage},
		},
	)
	if err != nil {
		c.log.Debug(fmt.Sprintf("Failed to list check runs: %v", err))
		return false, ""
	}

	return c.processCheckRuns(checkRuns.CheckRuns, handle, startTime)
}

// analyzeJobStatuses analyzes all jobs and returns status counts while updating completion state.
func (c *Client) analyzeJobStatuses(
	jobs []*JobInfo,
	allCompleted *bool,
	conclusion *string,
) map[string]int {
	jobsByStatus := make(map[string]int)

	for _, job := range jobs {
		jobsByStatus[job.Status]++

		// Update completion status
		switch job.Status {
		case statusInProgress, statusQueued:
			*allCompleted = false
		case statusCompleted:
			// Check conclusion for completed jobs
			switch job.Conclusion {
			case conclusionSuccess:
				// Success - no change needed
			case conclusionSkipped, conclusionNeutral:
				// Neutral/skipped - no change needed
			default:
				// Failed, cancelled, or other non-success conclusion
				if *conclusion == conclusionSuccess {
					*conclusion = job.Conclusion
				}
			}
		}
	}

	return jobsByStatus
}

// buildJobStatusMessage creates a status message from job statistics.
func (c *Client) buildJobStatusMessage(
	jobsByStatus map[string]int,
	allJobs []*JobInfo,
	elapsed time.Duration,
) string {
	var statusParts []string

	if count := jobsByStatus[statusInProgress]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d running", count))
	}
	if count := jobsByStatus[statusQueued]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d queued", count))
	}

	// Count completed jobs by conclusion
	succeeded, failed, skipped := c.countCompletedJobs(allJobs)

	if succeeded > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d passed", succeeded))
	}
	if failed > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d skipped", skipped))
	}

	statusMsg := fmt.Sprintf("Jobs: %s (total: %d) - %s",
		strings.Join(statusParts, ", "), len(allJobs), formatDuration(elapsed))

	// Add details for running/failed jobs
	jobDetails := c.collectJobDetails(allJobs)
	if len(jobDetails) > 0 {
		statusMsg += fmt.Sprintf(" [%s]", strings.Join(jobDetails, ", "))
	}

	return statusMsg
}

// countCompletedJobs counts completed jobs by their conclusion.
func (c *Client) countCompletedJobs(allJobs []*JobInfo) (int, int, int) {
	var succeeded, failed, skipped int
	for _, job := range allJobs {
		if job.Status == statusCompleted {
			switch job.Conclusion {
			case conclusionSuccess:
				succeeded++
			case conclusionSkipped, conclusionNeutral:
				skipped++
			default:
				failed++
			}
		}
	}
	return succeeded, failed, skipped
}

// collectJobDetails collects details for running or failed jobs (limited for readability).
func (c *Client) collectJobDetails(allJobs []*JobInfo) []string {
	var jobDetails []string
	for _, job := range allJobs {
		isRunning := job.Status == statusInProgress
		isFailed := job.Status == statusCompleted && job.Conclusion != conclusionSuccess &&
			job.Conclusion != conclusionSkipped && job.Conclusion != conclusionNeutral

		if isRunning || isFailed {
			detail := job.Name
			if isRunning && job.StartedAt != nil {
				duration := time.Since(*job.StartedAt)
				detail += fmt.Sprintf("(%s)", formatDuration(duration))
			}
			jobDetails = append(jobDetails, detail)
			if len(jobDetails) >= maxJobDetailsToDisplay {
				break
			}
		}
	}
	return jobDetails
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

// formatJobStatus formats a job/check status with icon and duration.
// Returns a formatted string like "⟳ build (running, 1m 23s)" or "✓ test (success, 45s)".
func formatJobStatus(job *JobInfo) string {
	if job == nil {
		return ""
	}

	// Select icon based on status and conclusion
	icon := iconPending
	statusText := job.Status

	if job.Status == statusCompleted {
		// Use conclusion for completed jobs
		statusText = job.Conclusion
		switch job.Conclusion {
		case conclusionSuccess:
			icon = iconSuccess
		case conclusionSkipped, conclusionNeutral:
			icon = iconSkipped
		case "cancelled":
			icon = iconCanceled
		default:
			// Failed, timed_out, action_required, etc.
			icon = iconFailed
		}
	} else {
		// Use status for in-progress or queued jobs
		switch job.Status {
		case statusInProgress:
			icon = iconRunning
			statusText = "running"
		case statusQueued:
			icon = iconPending
			statusText = "queued"
		}
	}

	// Calculate duration
	var durationStr string
	if job.Status == statusCompleted && job.StartedAt != nil && job.CompletedAt != nil {
		// Calculate duration for completed jobs
		duration := job.CompletedAt.Sub(*job.StartedAt)
		durationStr = formatDuration(duration)
	} else if job.Status == statusInProgress && job.StartedAt != nil {
		// Calculate elapsed time for running jobs
		elapsed := time.Since(*job.StartedAt)
		durationStr = formatDuration(elapsed)
	}

	// Format the complete status string
	if durationStr != "" {
		return fmt.Sprintf("%s %s (%s, %s)", icon, job.Name, statusText, durationStr)
	}
	return fmt.Sprintf("%s %s (%s)", icon, job.Name, statusText)
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

// newCheckTracker creates a new check tracker with initialized maps.
func newCheckTracker() *checkTracker {
	return &checkTracker{
		checks:  make(map[int64]*JobInfo),
		handles: make(map[int64]*bullets.BulletHandle),
	}
}

// getCheck retrieves a job/check by ID with read lock.
func (ct *checkTracker) getCheck(id int64) (*JobInfo, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	check, exists := ct.checks[id]
	return check, exists
}

// setCheck stores a job/check by ID with write lock.
func (ct *checkTracker) setCheck(id int64, check *JobInfo) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.checks[id] = check
}

// getHandle retrieves a bullet handle by job/check ID with read lock.
func (ct *checkTracker) getHandle(id int64) (*bullets.BulletHandle, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	handle, exists := ct.handles[id]
	return handle, exists
}

// setHandle stores a bullet handle for a job/check ID with write lock.
func (ct *checkTracker) setHandle(id int64, handle *bullets.BulletHandle) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.handles[id] = handle
}

// deleteCheck removes a job/check and its handle with write lock.
func (ct *checkTracker) deleteCheck(id int64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	delete(ct.checks, id)
	delete(ct.handles, id)
}

// update processes new jobs/checks, detects state transitions, and updates handles.
// Returns a list of state transition descriptions.
func (ct *checkTracker) update(newChecks []*JobInfo, logger *bullets.UpdatableLogger) []string {
	var transitions []string
	newCheckIDs := make(map[int64]bool)

	for _, newCheck := range newChecks {
		newCheckIDs[newCheck.ID] = true
		oldCheck, exists := ct.getCheck(newCheck.ID)

		if !exists {
			// New job detected - create handle with formatted status
			statusText := formatJobStatus(newCheck)
			handle := logger.InfoHandle(statusText)
			ct.setHandle(newCheck.ID, handle)
			ct.setCheck(newCheck.ID, newCheck)
			transitions = append(transitions, fmt.Sprintf("Job %d started: %s", newCheck.ID, newCheck.Name))
		} else if ct.hasStatusChanged(oldCheck, newCheck) {
			// Status or conclusion changed - update handle
			handle, handleExists := ct.getHandle(newCheck.ID)
			if handleExists {
				ct.updateHandleForCheck(handle, newCheck)
			}
			ct.setCheck(newCheck.ID, newCheck)
			transitions = append(transitions, ct.formatTransition(oldCheck, newCheck))
		} else {
			// No status change, just update check data (timestamps may have changed)
			ct.setCheck(newCheck.ID, newCheck)
			// Update handle to refresh duration for running jobs
			if newCheck.Status == statusInProgress {
				handle, handleExists := ct.getHandle(newCheck.ID)
				if handleExists {
					ct.updateHandleForCheck(handle, newCheck)
				}
			}
		}
	}

	// Detect removed jobs
	ct.mu.RLock()
	for id := range ct.checks {
		if !newCheckIDs[id] {
			transitions = append(transitions, fmt.Sprintf("Job %d removed", id))
		}
	}
	ct.mu.RUnlock()

	return transitions
}

// hasStatusChanged checks if job status or conclusion changed.
func (ct *checkTracker) hasStatusChanged(oldCheck, newCheck *JobInfo) bool {
	return oldCheck.Status != newCheck.Status || oldCheck.Conclusion != newCheck.Conclusion
}

// formatTransition creates a transition message for status changes.
func (ct *checkTracker) formatTransition(oldCheck, newCheck *JobInfo) string {
	oldState := oldCheck.Status
	if oldCheck.Status == statusCompleted && oldCheck.Conclusion != "" {
		oldState = oldCheck.Conclusion
	}

	newState := newCheck.Status
	if newCheck.Status == statusCompleted && newCheck.Conclusion != "" {
		newState = newCheck.Conclusion
	}

	return fmt.Sprintf("Job %d: %s -> %s", newCheck.ID, oldState, newState)
}

// updateHandleForCheck updates the bullet handle based on job status and conclusion using formatJobStatus.
func (ct *checkTracker) updateHandleForCheck(handle *bullets.BulletHandle, check *JobInfo) {
	statusText := formatJobStatus(check)

	// Handle completed jobs by conclusion
	if check.Status == statusCompleted {
		switch check.Conclusion {
		case conclusionSuccess:
			handle.Success(statusText)
		case conclusionSkipped, conclusionNeutral:
			handle.Update(bullets.InfoLevel, statusText)
		default:
			// Failed, cancelled, or other non-success conclusion
			handle.Error(statusText)
		}
		return
	}

	// Handle in-progress or queued jobs
	if check.Status == statusInProgress {
		// Use Pulse for animated spinner effect on running jobs (5s matches polling interval)
		handle.Pulse(5*time.Second, statusText)
	} else {
		handle.Update(bullets.InfoLevel, statusText)
	}
}

// cleanup removes completed jobs after a retention period.
func (ct *checkTracker) cleanup(retentionPeriod time.Duration) {
	now := time.Now()
	ct.mu.Lock()
	defer ct.mu.Unlock()

	for id, check := range ct.checks {
		if ct.shouldCleanupCheck(check, now, retentionPeriod) {
			delete(ct.checks, id)
			delete(ct.handles, id)
		}
	}
}

// shouldCleanupCheck determines if a job should be cleaned up based on its status and age.
func (ct *checkTracker) shouldCleanupCheck(check *JobInfo, now time.Time, retention time.Duration) bool {
	// Only cleanup completed jobs
	if check.Status != statusCompleted {
		return false
	}

	// Check if job is old enough to cleanup
	if check.CompletedAt != nil {
		return now.Sub(*check.CompletedAt) > retention
	}

	return false
}

// reset clears all tracked jobs and handles.
func (ct *checkTracker) reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.checks = make(map[int64]*JobInfo)
	ct.handles = make(map[int64]*bullets.BulletHandle)
}

// getActiveChecks returns jobs that are currently running or queued.
func (ct *checkTracker) getActiveChecks() []*JobInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var active []*JobInfo
	for _, check := range ct.checks {
		if check.Status == statusInProgress || check.Status == statusQueued {
			active = append(active, check)
		}
	}
	return active
}

// getFailedChecks returns jobs that have failed.
func (ct *checkTracker) getFailedChecks() []*JobInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var failed []*JobInfo
	for _, check := range ct.checks {
		if check.Status == statusCompleted && check.Conclusion != conclusionSuccess &&
			check.Conclusion != conclusionSkipped && check.Conclusion != conclusionNeutral {
			failed = append(failed, check)
		}
	}
	return failed
}

// getAllChecks returns a copy of all tracked jobs.
func (ct *checkTracker) getAllChecks() []*JobInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	checks := make([]*JobInfo, 0, len(ct.checks))
	for _, check := range ct.checks {
		checks = append(checks, check)
	}
	return checks
}