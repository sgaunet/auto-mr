// Package github provides a GitHub API client for pull request lifecycle management.
//
// The package handles:
//   - Creating and fetching pull requests with assignees, reviewers, and labels
//   - Waiting for GitHub Actions workflow completion with real-time job-level visualization
//   - Merging pull requests (merge, squash, or rebase strategies)
//   - Deleting remote branches after merge
//   - Label retrieval for interactive selection
//
// Authentication requires a GITHUB_TOKEN environment variable containing a
// personal access token with repo scope.
//
// Usage:
//
//	client, err := github.NewClient()
//	client.SetLogger(logger)
//	client.SetRepositoryFromURL("https://github.com/owner/repo.git")
//	labels, _ := client.ListLabels()
//	pr, _ := client.CreatePullRequest("feature", "main", "Title", "Body", []string{"user"}, []string{"reviewer"}, nil)
//
// Thread Safety: [Client] is not safe for concurrent use. The workflow waiting
// methods use internal goroutines but the Client itself should be used from
// a single goroutine.
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
	"github.com/sgaunet/auto-mr/internal/polling"
	"github.com/sgaunet/auto-mr/internal/timeutil"
	"github.com/sgaunet/bullets"
	"golang.org/x/oauth2"
)

// NewClient creates a new GitHub client authenticated via the GITHUB_TOKEN environment variable.
//
// Returns [ErrTokenRequired] if GITHUB_TOKEN is not set.
func NewClient(ctx context.Context) (*Client, error) {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		return nil, errTokenRequired
	}

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

// WaitForWorkflows waits for all GitHub Actions workflow runs to complete for the pull request.
// It polls at 5-second intervals and displays real-time job-level progress with animated spinners.
// If no workflows are configured, it returns "success" immediately.
//
// Parameters:
//   - timeout: maximum wait duration (typically 1m to 8h)
//
// Returns the overall conclusion ("success", "failure", "cancelled", etc.).
// Returns [ErrWorkflowTimeout] if the timeout is exceeded.
//
// A pull request must have been created or fetched before calling this method.
func (c *Client) WaitForWorkflows(ctx context.Context, timeout time.Duration) (string, error) {
	c.log.Debug(fmt.Sprintf("Waiting for workflows, timeout: %v", timeout))
	start := time.Now()

	// The overall budget bounds the whole wait; each request additionally gets a short
	// deadline of its own, so one stalled response cannot consume the entire budget.
	// Only overallCtx can tell "this request stalled, poll again" from "the budget is
	// spent", because a per-request context derived from it fails in both cases.
	overallCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// First check if any workflow runs are expected for this PR
	if !c.hasWorkflowRuns(overallCtx) {
		c.log.Info("No workflow runs configured for this pull request, proceeding without checks")
		return conclusionSuccess, nil
	}

	// Create updatable handle for workflow status
	c.display.Info("Waiting for workflows to complete...")
	if !polling.Sleep(overallCtx, workflowCreationDelay) { // Let the time to workflows to be created
		return "", waitOutcome(overallCtx)
	}
	c.display.IncreasePadding()
	defer c.display.DecreasePadding()

	// Initialize check tracker for managing individual job handles
	tracker := newCheckTracker(overallCtx)
	defer tracker.Stop()

	for overallCtx.Err() == nil {
		conclusion, done, err := c.pollWorkflowsOnce(overallCtx, tracker)
		if err != nil {
			if overallCtx.Err() != nil {
				break // Budget spent or cancelled; reported after the loop.
			}
			if !isTransient(err) {
				return "", err
			}
			// Rate limits and server-side faults are expected over a long wait; the
			// next poll retries rather than abandoning a request whose CI may be
			// about to pass.
			c.log.Debug(fmt.Sprintf("Transient error while polling, will retry: %v", err))
		} else if done {
			c.reportWorkflowOutcome(conclusion, time.Since(start))
			return conclusion, nil
		}
		if !polling.Sleep(overallCtx, polling.DefaultSchedule.IntervalFor(time.Since(start))) {
			break
		}
	}

	totalDuration := time.Since(start)
	if errors.Is(overallCtx.Err(), context.Canceled) {
		c.display.Error("Cancelled after " + timeutil.FormatDuration(totalDuration))
		return "", errWorkflowCanceled
	}
	c.display.Error("Timeout after " + timeutil.FormatDuration(totalDuration))
	return "", errWorkflowTimeout
}

// waitOutcome maps a finished wait context to the sentinel that describes why it
// ended, so callers can distinguish a deliberate interrupt from a spent budget.
func waitOutcome(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return errWorkflowCanceled
	}
	return errWorkflowTimeout
}

// pollWorkflowsOnce performs a single polling round, bounded by its own short
// deadline so a stalled request cannot consume the overall budget.
//
// It reports the overall conclusion and whether every workflow has reached a
// terminal state. An error accompanied by a finished overallCtx means the wait
// itself ended rather than the request failing, and the caller distinguishes those.
func (c *Client) pollWorkflowsOnce(
	overallCtx context.Context, tracker *checkTracker,
) (string, bool, error) {
	pollCtx, cancelPoll := context.WithTimeout(overallCtx, polling.PerCallTimeout)
	defer cancelPoll()

	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(
		pollCtx, c.owner, c.repo, c.prSHA,
		&github.ListCheckRunsOptions{
			ListOptions: github.ListOptions{PerPage: maxCheckRunsPerPage},
		},
	)
	if err != nil {
		if overallCtx.Err() == nil && !isTransient(err) {
			c.display.Error(fmt.Sprintf("Failed to list check runs: %v", err))
		}
		return "", false, fmt.Errorf("failed to list check runs: %w", err)
	}

	// Wait silently for workflows to appear; they show as individual spinners once
	// they start.
	if checkRuns.GetTotal() == 0 {
		return "", false, nil
	}

	allCompleted, conclusion := c.processWorkflowsWithJobTracking(pollCtx, tracker, checkRuns)
	return conclusion, allCompleted, nil
}

// reportWorkflowOutcome renders the final summary line for finished workflows.
func (c *Client) reportWorkflowOutcome(conclusion string, elapsed time.Duration) {
	if conclusion == conclusionSuccess {
		c.display.Success("Workflows completed successfully - total time: " +
			timeutil.FormatDuration(elapsed))
		return
	}

	msg := "Workflows failed - total time: " + timeutil.FormatDuration(elapsed)
	handle := c.display.InfoHandle(msg)
	handle.Error(msg)
}

// processWorkflowsWithJobTracking processes workflows using checkTracker for individual job display.
func (c *Client) processWorkflowsWithJobTracking(
	ctx context.Context, tracker *checkTracker, checkRuns *github.ListCheckRunsResults,
) (bool, string) {
	// Try to fetch workflow jobs
	jobs, err := c.fetchWorkflowJobs(ctx)
	if err != nil {
		c.log.Debug(fmt.Sprintf("Failed to fetch workflow jobs, falling back to check runs: %v", err))
		return c.checkRunsFallback(tracker, checkRuns)
	}

	// If no jobs found, fall back to check runs
	if len(jobs) == 0 {
		c.log.Debug("No workflow jobs found, falling back to check runs")
		return c.checkRunsFallback(tracker, checkRuns)
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
func (c *Client) checkRunsFallback(
	tracker *checkTracker, checkRuns *github.ListCheckRunsResults,
) (bool, string) {
	if checkRuns == nil || checkRuns.GetTotal() == 0 {
		return false, ""
	}
	return c.processCheckRunsFallback(tracker, checkRuns.CheckRuns)
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

// GetMergeMethod returns the appropriate merge method string for the GitHub API.
// Returns "squash" if squash is true, otherwise "merge".
func GetMergeMethod(squash bool) string {
	if squash {
		return "squash"
	}
	return "merge"
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
		return timeutil.FormatDuration(duration)
	}
	if job.Status == statusInProgress && job.StartedAt != nil {
		elapsed := time.Since(*job.StartedAt)
		return timeutil.FormatDuration(elapsed)
	}
	return ""
}

// isTransient reports whether an error from the GitHub API is worth retrying.
//
// go-github models rate limiting with dedicated types and everything else with
// ErrorResponse, so both shapes have to be inspected to recover the status code.
func isTransient(err error) bool {
	var rateLimit *github.RateLimitError
	if errors.As(err, &rateLimit) {
		return true
	}

	var abuse *github.AbuseRateLimitError
	if errors.As(err, &abuse) {
		return true
	}

	var errResp *github.ErrorResponse
	if errors.As(err, &errResp) && errResp.Response != nil {
		return polling.Transient(errResp.Response.StatusCode)
	}
	return false
}
