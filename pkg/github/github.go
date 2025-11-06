// Package github provides GitHub API client operations.
package github

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/bullets"
	"golang.org/x/oauth2"
)


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

	log := logger.NoLogger()
	updatable := bullets.NewUpdatable(os.Stdout)
	display := newDisplayRenderer(log, updatable)

	return &Client{
		client:  client,
		log:     log,
		display: display,
	}, nil
}

// SetLogger sets the logger for the GitHub client.
func (c *Client) SetLogger(logger *bullets.Logger) {
	c.log = logger
	c.display.SetLogger(logger)
	c.log.Debug("GitHub client logger configured")
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
	c.display.Info("Waiting for workflows to complete...")
	c.display.IncreasePadding()
	defer c.display.DecreasePadding()

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
			c.display.Error(fmt.Sprintf("Failed to list check runs: %v", err))
			return "", fmt.Errorf("failed to list check runs: %w", err)
		}

		if checkRuns.GetTotal() == 0 {
			// Wait silently for workflows to appear (they'll show as individual spinners when they start)
			time.Sleep(checkPollInterval)
			continue
		}

		// Try to fetch and display job-level information with check tracker
		allCompleted, conclusion := c.processWorkflowsWithJobTracking(tracker)

		if !allCompleted {
			time.Sleep(checkPollInterval)
			continue
		}

		// All workflows completed - display final summary
		totalDuration := time.Since(start)
		if conclusion == conclusionSuccess {
			c.display.Success("Workflows completed successfully - total time: " +
				formatDuration(totalDuration))
		} else {
			msg := "Workflows failed - total time: " +
				formatDuration(totalDuration)
			handle := c.display.InfoHandle(msg)
			handle.Error(msg)
		}
		return conclusion, nil
	}

	totalDuration := time.Since(start)
	c.display.Error("Timeout after " + formatDuration(totalDuration))
	return "", errWorkflowTimeout
}

// processWorkflowsWithJobTracking processes workflows using checkTracker for individual job display.
func (c *Client) processWorkflowsWithJobTracking(tracker *checkTracker) (bool, string) {
	// Try to fetch workflow jobs
	jobs, err := c.fetchWorkflowJobs()
	if err != nil {
		c.log.Debug(fmt.Sprintf("Failed to fetch workflow jobs, falling back to check runs: %v", err))
		return c.fallbackToCheckRuns(tracker)
	}

	// If no jobs found, fall back to check runs
	if len(jobs) == 0 {
		c.log.Debug("No workflow jobs found, falling back to check runs")
		return c.fallbackToCheckRuns(tracker)
	}

	// Update check tracker with new jobs (creates/updates handles automatically)
	transitions := tracker.update(jobs, c.display.GetUpdatable())
	for _, transition := range transitions {
		c.log.Debug(transition)
	}

	// Analyze job statuses for completion
	return c.analyzeJobCompletion(jobs)
}

// fallbackToCheckRuns attempts to fall back to check runs API.
func (c *Client) fallbackToCheckRuns(tracker *checkTracker) (bool, string) {
	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(
		c.ctx(), c.owner, c.repo, c.prSHA,
		&github.ListCheckRunsOptions{
			ListOptions: github.ListOptions{PerPage: maxCheckRunsPerPage},
		},
	)
	if err == nil && checkRuns.GetTotal() > 0 {
		return c.processCheckRunsFallback(tracker, checkRuns.CheckRuns)
	}
	return false, ""
}

// analyzeJobCompletion checks if all jobs are completed and determines overall conclusion.
func (c *Client) analyzeJobCompletion(jobs []*JobInfo) (bool, string) {
	allCompleted := true
	conclusion := conclusionSuccess

	for _, job := range jobs {
		switch job.Status {
		case statusInProgress, statusQueued:
			allCompleted = false
		case statusCompleted:
			if job.Conclusion != conclusionSuccess && job.Conclusion != conclusionSkipped &&
				job.Conclusion != conclusionNeutral && conclusion == conclusionSuccess {
				conclusion = job.Conclusion
			}
		}
	}

	return allCompleted, conclusion
}

// processCheckRunsFallback processes check runs using checkTracker for individual spinners.
// This is used as a fallback when workflow jobs API is unavailable.
func (c *Client) processCheckRunsFallback(tracker *checkTracker, checkRuns []*github.CheckRun) (bool, string) {
	// Convert CheckRuns to JobInfo format for tracker
	jobs := c.convertCheckRunsToJobInfo(checkRuns)

	// Update check tracker with converted jobs (creates/updates spinners automatically)
	transitions := tracker.update(jobs, c.display.GetUpdatable())
	for _, transition := range transitions {
		c.log.Debug(transition)
	}

	// Analyze completion status
	return c.analyzeJobCompletion(jobs)
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

// formatJobStatus formats a job/check status with duration.
// Returns a formatted string like "build (running, 1m 23s)" or "test (success, 45s)".
// Icons are added by the bullets library methods (Success/Error/etc), not by this function.
func formatJobStatus(job *JobInfo) string {
	if job == nil {
		return ""
	}

	statusText := getJobStatusText(job)
	durationStr := calculateJobDuration(job)

	// Format the complete status string (without icon - bullets library adds those)
	if durationStr != "" {
		return fmt.Sprintf("%s (%s, %s)", job.Name, statusText, durationStr)
	}
	return fmt.Sprintf("%s (%s)", job.Name, statusText)
}

// getJobStatusText returns the appropriate status text for a job.
func getJobStatusText(job *JobInfo) string {
	switch job.Status {
	case statusCompleted:
		return job.Conclusion
	case statusInProgress:
		return "running"
	case statusQueued:
		return "queued"
	default:
		return job.Status
	}
}

// calculateJobDuration calculates the duration string for a job.
func calculateJobDuration(job *JobInfo) string {
	if job.Status == statusCompleted && job.StartedAt != nil && job.CompletedAt != nil {
		duration := job.CompletedAt.Sub(*job.StartedAt)
		return formatDuration(duration)
	}
	if job.Status == statusInProgress && job.StartedAt != nil {
		elapsed := time.Since(*job.StartedAt)
		return formatDuration(elapsed)
	}
	return ""
}

