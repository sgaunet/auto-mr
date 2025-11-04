// Package gitlab provides GitLab API client operations.
package gitlab

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
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
	minURLParts            = 2
	pipelinePollInterval   = 5 * time.Second
	maxJobDetailsToDisplay = 3
	statusSuccess          = "success"
	statusRunning          = "running"
	statusPending          = "pending"
	statusCreated          = "created"
	statusFailed           = "failed"
	statusCanceled         = "canceled"
	statusSkipped          = "skipped"

	// Status icons for visual representation
	iconRunning  = "⟳"
	iconPending  = "○"
	iconSuccess  = "✓"
	iconFailed   = "✗"
	iconCanceled = "⊘"
	iconSkipped  = "○"
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

// Job represents a GitLab pipeline job with detailed status information.
type Job struct {
	ID         int
	Name       string
	Status     string
	Stage      string
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
	Duration   float64
	WebURL     string
}

// jobTracker tracks jobs and their display handles with thread-safe access.
type jobTracker struct {
	mu      sync.RWMutex
	jobs    map[int]*Job
	handles map[int]*bullets.BulletHandle
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
	labels []string, squash bool,
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
		Squash:             gitlab.Ptr(squash),
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
		return statusSuccess, nil
	}

	// Create updatable handle for pipeline status
	c.updatableLog.Info("Waiting for pipelines to complete...")
	c.updatableLog.IncreasePadding()
	defer c.updatableLog.DecreasePadding()

	// Initialize job tracker for managing individual job handles
	tracker := newJobTracker()

	for time.Since(start) < timeout {
		pipelines, _, err := c.client.MergeRequests.ListMergeRequestPipelines(c.projectID, c.mrIID, nil)
		if err != nil {
			c.updatableLog.Error(fmt.Sprintf("Failed to list MR pipelines: %v", err))
			return "", fmt.Errorf("failed to list MR pipelines: %w", err)
		}

		if len(pipelines) == 0 {
			elapsed := time.Since(start)
			c.updatableLog.Info(fmt.Sprintf("Waiting for CI to start... (%s)", formatDuration(elapsed)))
			time.Sleep(pipelinePollInterval)
			continue
		}

		// Process all pipelines with individual job tracking
		allCompleted, overallStatus := c.processPipelinesWithJobTracking(pipelines, tracker, start)

		if !allCompleted {
			time.Sleep(pipelinePollInterval)
			continue
		}

		// All pipelines completed - display final summary
		totalDuration := time.Since(start)
		if overallStatus == statusSuccess {
			c.updatableLog.Success(fmt.Sprintf("%s Pipeline completed successfully - total time: %s",
				iconSuccess, formatDuration(totalDuration)))
		} else {
			msg := fmt.Sprintf("%s Pipeline failed - total time: %s",
				iconFailed, formatDuration(totalDuration))
			handle := c.updatableLog.InfoHandle(msg)
			handle.Error(msg)
		}
		return overallStatus, nil
	}

	totalDuration := time.Since(start)
	c.updatableLog.Error("Timeout after " + formatDuration(totalDuration))
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
func (c *Client) MergeMergeRequest(mrIID int, squash bool) error {
	c.log.Debug(fmt.Sprintf("Merging merge request, IID: %d", mrIID))
	mergeOptions := &gitlab.AcceptMergeRequestOptions{
		Squash:             gitlab.Ptr(squash),
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

// pipelineStats holds counters for pipeline statuses.
type pipelineStats struct {
	running          int
	pending          int
	created          int
	success          int
	failed           int
	canceled         int
	skipped          int
	runningPipelines []string
}

// processPipelinesWithJobTracking evaluates all pipeline statuses using jobTracker for individual job display.
func (c *Client) processPipelinesWithJobTracking(
	pipelines []*gitlab.PipelineInfo, tracker *jobTracker, startTime time.Time,
) (bool, string) {
	allCompleted := true
	overallStatus := statusSuccess

	// Fetch jobs for all pipelines in parallel for better performance with multiple pipelines
	type pipelineJobs struct {
		pipelineID int
		jobs       []*Job
		err        error
	}

	resultChan := make(chan pipelineJobs, len(pipelines))
	var wg sync.WaitGroup

	// Launch goroutines to fetch jobs concurrently
	for _, pipeline := range pipelines {
		wg.Add(1)
		go func(p *gitlab.PipelineInfo) {
			defer wg.Done()
			jobs, err := c.fetchPipelineJobs(p.ID)
			resultChan <- pipelineJobs{
				pipelineID: p.ID,
				jobs:       jobs,
				err:        err,
			}
		}(pipeline)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect all jobs from concurrent fetches
	var allJobs []*Job
	for result := range resultChan {
		if result.err != nil {
			c.log.Debug(fmt.Sprintf("Failed to fetch jobs for pipeline %d: %v", result.pipelineID, result.err))
			// Find the pipeline for fallback processing
			for _, p := range pipelines {
				if p.ID == result.pipelineID {
					c.processPipelineStatus(p, &pipelineStats{}, &allCompleted, &overallStatus)
					break
				}
			}
			continue
		}
		allJobs = append(allJobs, result.jobs...)
	}

	// If no jobs found, fall back to aggregate view
	if len(allJobs) == 0 {
		stats := c.collectPipelineStats(pipelines, &allCompleted, &overallStatus)
		elapsed := time.Since(startTime)
		statusMsg := c.buildPipelineStatusMessage(stats, len(pipelines), elapsed)
		c.updatableLog.Info(statusMsg)
		return allCompleted, overallStatus
	}

	// Update job tracker with new jobs (creates/updates handles automatically)
	tracker.update(allJobs, c.updatableLog)

	// Analyze job statuses for completion
	for _, job := range allJobs {
		switch job.Status {
		case statusRunning, statusPending, statusCreated:
			allCompleted = false
		case statusFailed:
			if overallStatus == statusSuccess {
				overallStatus = statusFailed
			}
		case statusCanceled:
			if overallStatus == statusSuccess {
				overallStatus = statusCanceled
			}
		}
	}

	return allCompleted, overallStatus
}

// processPipelines evaluates all pipeline statuses and returns completion state and overall status.
func (c *Client) processPipelines(
	pipelines []*gitlab.PipelineInfo, handle *bullets.BulletHandle, startTime time.Time,
) (bool, string) {
	allCompleted := true
	overallStatus := statusSuccess
	elapsed := time.Since(startTime)

	// Try to fetch and display job-level information
	jobsDisplayed := c.displayPipelineJobs(pipelines, handle, &allCompleted, &overallStatus, elapsed)

	// Fallback to aggregate view if job fetching failed
	if !jobsDisplayed {
		stats := c.collectPipelineStats(pipelines, &allCompleted, &overallStatus)
		statusMsg := c.buildPipelineStatusMessage(stats, len(pipelines), elapsed)
		handle.Update(bullets.InfoLevel, statusMsg)
	}

	return allCompleted, overallStatus
}

// collectPipelineStats collects statistics from all pipelines.
func (c *Client) collectPipelineStats(
	pipelines []*gitlab.PipelineInfo, allCompleted *bool, overallStatus *string,
) pipelineStats {
	stats := pipelineStats{}

	for _, pipeline := range pipelines {
		c.processPipelineStatus(pipeline, &stats, allCompleted, overallStatus)
	}

	return stats
}

// processPipelineStatus processes a single pipeline's status.
func (c *Client) processPipelineStatus(
	pipeline *gitlab.PipelineInfo, stats *pipelineStats, allCompleted *bool, overallStatus *string,
) {
	status := pipeline.Status

	switch status {
	case statusRunning:
		c.handleRunningPipeline(pipeline, stats, allCompleted)
	case statusPending, statusCreated:
		c.handlePendingPipeline(status, stats, allCompleted)
	case statusSuccess:
		stats.success++
	case statusFailed:
		c.handleFailedPipeline(stats, overallStatus)
	case statusCanceled:
		c.handleCanceledPipeline(stats, overallStatus)
	case statusSkipped:
		stats.skipped++
	default:
		*allCompleted = false
	}
}

// handlePendingPipeline handles a pending or created pipeline.
func (c *Client) handlePendingPipeline(status string, stats *pipelineStats, allCompleted *bool) {
	*allCompleted = false
	if status == statusPending {
		stats.pending++
	} else {
		stats.created++
	}
}

// handleFailedPipeline handles a failed pipeline.
func (c *Client) handleFailedPipeline(stats *pipelineStats, overallStatus *string) {
	stats.failed++
	if *overallStatus == statusSuccess {
		*overallStatus = statusFailed
	}
}

// handleCanceledPipeline handles a canceled pipeline.
func (c *Client) handleCanceledPipeline(stats *pipelineStats, overallStatus *string) {
	stats.canceled++
	if *overallStatus == statusSuccess {
		*overallStatus = statusCanceled
	}
}

// handleRunningPipeline handles a running pipeline.
func (c *Client) handleRunningPipeline(
	pipeline *gitlab.PipelineInfo, stats *pipelineStats, allCompleted *bool,
) {
	*allCompleted = false
	stats.running++
	if pipeline.Ref != "" {
		stats.runningPipelines = append(stats.runningPipelines, fmt.Sprintf("#%d", pipeline.ID))
	}
}

// buildPipelineStatusMessage builds a status message from pipeline statistics.
func (c *Client) buildPipelineStatusMessage(stats pipelineStats, total int, elapsed time.Duration) string {
	var statusParts []string

	if stats.running > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d running", stats.running))
	}
	if stats.pending > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d pending", stats.pending))
	}
	if stats.created > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d created", stats.created))
	}
	if stats.success > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d passed", stats.success))
	}
	if stats.failed > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d failed", stats.failed))
	}
	if stats.canceled > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d canceled", stats.canceled))
	}
	if stats.skipped > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d skipped", stats.skipped))
	}

	statusMsg := fmt.Sprintf("Pipelines: %s (total: %d) - %s",
		strings.Join(statusParts, ", "), total, formatDuration(elapsed))

	// Add currently running pipeline IDs for context
	if len(stats.runningPipelines) > 0 && len(stats.runningPipelines) <= 3 {
		statusMsg += fmt.Sprintf(" [%s]", strings.Join(stats.runningPipelines, ", "))
	}

	return statusMsg
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

// displayPipelineJobs fetches and displays individual job statuses for all pipelines.
// Returns true if jobs were successfully displayed, false to trigger fallback to aggregate view.
func (c *Client) displayPipelineJobs(
	pipelines []*gitlab.PipelineInfo,
	handle *bullets.BulletHandle,
	allCompleted *bool,
	overallStatus *string,
	elapsed time.Duration,
) bool {
	if len(pipelines) == 0 {
		return false
	}

	// Track job statistics across all pipelines
	var allJobs []*Job
	jobsByStatus := make(map[string]int)

	// Fetch jobs for all pipelines
	for _, pipeline := range pipelines {
		jobs, err := c.fetchPipelineJobs(pipeline.ID)
		if err != nil {
			c.log.Debug(fmt.Sprintf(
				"Failed to fetch jobs for pipeline %d, falling back to aggregate view: %v",
				pipeline.ID, err))
			return false
		}
		allJobs = append(allJobs, jobs...)
	}

	// If no jobs found, fall back to aggregate view
	if len(allJobs) == 0 {
		c.log.Debug("No jobs found in pipelines, falling back to aggregate view")
		return false
	}

	// Analyze job statuses and count by status
	c.analyzeJobStatuses(allJobs, jobsByStatus, allCompleted, overallStatus)

	// Build status message showing job-level details
	statusMsg := c.buildJobStatusMessage(jobsByStatus, allJobs, elapsed)
	handle.Update(bullets.InfoLevel, statusMsg)

	return true
}

// analyzeJobStatuses analyzes all jobs and updates completion status and counts.
func (c *Client) analyzeJobStatuses(
	allJobs []*Job,
	jobsByStatus map[string]int,
	allCompleted *bool,
	overallStatus *string,
) {
	for _, job := range allJobs {
		jobsByStatus[job.Status]++

		// Update completion status
		switch job.Status {
		case statusRunning, statusPending, statusCreated:
			*allCompleted = false
		case statusFailed:
			if *overallStatus == statusSuccess {
				*overallStatus = statusFailed
			}
		case statusCanceled:
			if *overallStatus == statusSuccess {
				*overallStatus = statusCanceled
			}
		}
	}
}

// buildJobStatusMessage creates a status message from job statistics.
func (c *Client) buildJobStatusMessage(jobsByStatus map[string]int, allJobs []*Job, elapsed time.Duration) string {
	var statusParts []string

	if count := jobsByStatus[statusRunning]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d running", count))
	}
	if count := jobsByStatus[statusPending]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d pending", count))
	}
	if count := jobsByStatus[statusCreated]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d created", count))
	}
	if count := jobsByStatus[statusSuccess]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d passed", count))
	}
	if count := jobsByStatus[statusFailed]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d failed", count))
	}
	if count := jobsByStatus[statusCanceled]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d canceled", count))
	}
	if count := jobsByStatus[statusSkipped]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d skipped", count))
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

// collectJobDetails collects details for running or failed jobs (limited for readability).
func (c *Client) collectJobDetails(allJobs []*Job) []string {
	var jobDetails []string
	for _, job := range allJobs {
		if job.Status == statusRunning || job.Status == statusFailed {
			detail := fmt.Sprintf("%s:%s", job.Stage, job.Name)
			if job.Status == statusRunning && job.Duration > 0 {
				detail += fmt.Sprintf("(%s)", formatDuration(time.Duration(job.Duration)*time.Second))
			}
			jobDetails = append(jobDetails, detail)
			if len(jobDetails) >= maxJobDetailsToDisplay {
				break
			}
		}
	}
	return jobDetails
}

// fetchPipelineJobs fetches all jobs for a given pipeline with pagination support.
func (c *Client) fetchPipelineJobs(pipelineID int) ([]*Job, error) {
	c.log.Debug(fmt.Sprintf("Fetching jobs for pipeline %d", pipelineID))

	var allJobs []*Job
	page := 1
	perPage := 100

	for {
		jobs, resp, err := c.client.Jobs.ListPipelineJobs(
			c.projectID,
			pipelineID,
			&gitlab.ListJobsOptions{
				ListOptions: gitlab.ListOptions{
					Page:    page,
					PerPage: perPage,
				},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list pipeline jobs: %w", err)
		}

		// Convert GitLab jobs to our Job struct
		for _, glJob := range jobs {
			job := &Job{
				ID:         glJob.ID,
				Name:       glJob.Name,
				Status:     glJob.Status,
				Stage:      glJob.Stage,
				CreatedAt:  *glJob.CreatedAt,
				StartedAt:  glJob.StartedAt,
				FinishedAt: glJob.FinishedAt,
				Duration:   glJob.Duration,
				WebURL:     glJob.WebURL,
			}
			allJobs = append(allJobs, job)
		}

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	c.log.Debug(fmt.Sprintf("Fetched %d jobs for pipeline %d", len(allJobs), pipelineID))
	return allJobs, nil
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

// formatJobStatus formats a job status with icon and duration.
// Returns a formatted string like "⟳ build (running, 1m 23s)" or "✓ test (success, 45s)".
func formatJobStatus(job *Job) string {
	if job == nil {
		return ""
	}

	// Select icon based on status
	icon := iconPending
	switch job.Status {
	case statusRunning:
		icon = iconRunning
	case statusSuccess:
		icon = iconSuccess
	case statusFailed:
		icon = iconFailed
	case statusCanceled:
		icon = iconCanceled
	case statusSkipped:
		icon = iconSkipped
	case statusPending, statusCreated:
		icon = iconPending
	}

	// Build job name with stage prefix if available
	jobName := job.Name
	if job.Stage != "" {
		jobName = job.Stage + "/" + job.Name
	}

	// Calculate duration
	var durationStr string
	if job.Duration > 0 {
		durationStr = formatDuration(time.Duration(job.Duration) * time.Second)
	} else if job.StartedAt != nil && job.Status == statusRunning {
		// Calculate elapsed time for running jobs
		elapsed := time.Since(*job.StartedAt)
		durationStr = formatDuration(elapsed)
	}

	// Format the complete status string
	if durationStr != "" {
		return fmt.Sprintf("%s %s (%s, %s)", icon, jobName, job.Status, durationStr)
	}
	return fmt.Sprintf("%s %s (%s)", icon, jobName, job.Status)
}

// newJobTracker creates a new job tracker with initialized maps.
func newJobTracker() *jobTracker {
	return &jobTracker{
		jobs:    make(map[int]*Job),
		handles: make(map[int]*bullets.BulletHandle),
	}
}

// getJob retrieves a job by ID with read lock.
func (jt *jobTracker) getJob(id int) (*Job, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	job, exists := jt.jobs[id]
	return job, exists
}

// setJob stores a job by ID with write lock.
func (jt *jobTracker) setJob(id int, job *Job) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.jobs[id] = job
}

// getHandle retrieves a bullet handle by job ID with read lock.
func (jt *jobTracker) getHandle(id int) (*bullets.BulletHandle, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	handle, exists := jt.handles[id]
	return handle, exists
}

// setHandle stores a bullet handle for a job ID with write lock.
func (jt *jobTracker) setHandle(id int, handle *bullets.BulletHandle) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.handles[id] = handle
}

// deleteJob removes a job and its handle with write lock.
func (jt *jobTracker) deleteJob(id int) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	delete(jt.jobs, id)
	delete(jt.handles, id)
}

// update processes new jobs, detects state transitions, and updates handles.
// Returns a list of state transition descriptions.
func (jt *jobTracker) update(newJobs []*Job, logger *bullets.UpdatableLogger) []string {
	var transitions []string
	newJobIDs := make(map[int]bool)

	for _, newJob := range newJobs {
		// Skip nil jobs or jobs with invalid IDs
		if newJob == nil || newJob.ID == 0 {
			continue
		}

		// Handle duplicate job IDs - only process first occurrence
		if newJobIDs[newJob.ID] {
			continue
		}

		newJobIDs[newJob.ID] = true
		oldJob, exists := jt.getJob(newJob.ID)

		if !exists {
			// New job detected - create handle with formatted status
			statusText := formatJobStatus(newJob)
			handle := logger.InfoHandle(statusText)
			jt.setHandle(newJob.ID, handle)
			jt.setJob(newJob.ID, newJob)
			// Start pulse animation for running jobs
			if newJob.Status == statusRunning {
				handle.Pulse(5*time.Second, statusText)
			}
			transitions = append(transitions, fmt.Sprintf("Job %d started: %s/%s", newJob.ID, newJob.Stage, newJob.Name))
		} else if oldJob.Status != newJob.Status {
			// Status changed - update display and handle pulse animation
			wasPulsing := oldJob.Status == statusRunning
			isPulsing := newJob.Status == statusRunning

			jt.updateHandleForJob(logger, newJob, wasPulsing, isPulsing)
			jt.setJob(newJob.ID, newJob)
			transitions = append(transitions, fmt.Sprintf("Job %d: %s -> %s", newJob.ID, oldJob.Status, newJob.Status))
		} else {
			// No status change, just update job data (timestamps/duration may have changed)
			jt.setJob(newJob.ID, newJob)
			// Update text for running jobs to show elapsed time (without re-pulsing)
			if newJob.Status == statusRunning && newJob.StartedAt != nil {
				if handle, exists := jt.getHandle(newJob.ID); exists {
					statusText := formatJobStatus(newJob)
					handle.Update(bullets.InfoLevel, statusText)
				}
			}
		}
	}

	// Detect removed jobs
	jt.mu.RLock()
	for id := range jt.jobs {
		if !newJobIDs[id] {
			transitions = append(transitions, fmt.Sprintf("Job %d removed", id))
		}
	}
	jt.mu.RUnlock()

	return transitions
}

// updateHandleForJob updates the display for a job when status changes.
// wasPulsing and isPulsing control whether to start or stop the pulse animation.
func (jt *jobTracker) updateHandleForJob(logger *bullets.UpdatableLogger, job *Job, wasPulsing, isPulsing bool) {
	statusText := formatJobStatus(job)

	if handle, exists := jt.getHandle(job.ID); exists {
		switch job.Status {
		case statusSuccess:
			handle.Success(statusText)
		case statusFailed:
			handle.Error(statusText)
		case statusCanceled:
			handle.Warning(statusText)
		case statusSkipped:
			handle.Update(bullets.InfoLevel, statusText)
		case statusRunning:
			handle.Update(bullets.InfoLevel, statusText)
			// Only start pulse animation when transitioning TO running status
			if isPulsing && !wasPulsing {
				handle.Pulse(5*time.Second, statusText)
			}
		case statusPending, statusCreated:
			handle.Update(bullets.InfoLevel, statusText)
		default:
			handle.Update(bullets.InfoLevel, statusText)
		}
	}
}

// cleanup removes completed, failed, canceled, or skipped jobs after a timeout.
func (jt *jobTracker) cleanup(retentionPeriod time.Duration) {
	now := time.Now()
	jt.mu.Lock()
	defer jt.mu.Unlock()

	for id, job := range jt.jobs {
		if jt.shouldCleanupJob(job, now, retentionPeriod) {
			delete(jt.jobs, id)
			delete(jt.handles, id)
		}
	}
}

// shouldCleanupJob determines if a job should be cleaned up based on its status and age.
func (jt *jobTracker) shouldCleanupJob(job *Job, now time.Time, retention time.Duration) bool {
	// Only cleanup completed, failed, canceled, or skipped jobs
	if job.Status != statusSuccess && job.Status != statusFailed &&
		job.Status != statusCanceled && job.Status != statusSkipped {
		return false
	}

	// Check if job is old enough to cleanup
	if job.FinishedAt != nil {
		return now.Sub(*job.FinishedAt) > retention
	}

	return false
}

// reset clears all tracked jobs and handles.
func (jt *jobTracker) reset() {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.jobs = make(map[int]*Job)
	jt.handles = make(map[int]*bullets.BulletHandle)
}

// getActiveJobs returns jobs that are currently running or pending.
func (jt *jobTracker) getActiveJobs() []*Job {
	jt.mu.RLock()
	defer jt.mu.RUnlock()

	var active []*Job
	for _, job := range jt.jobs {
		if job.Status == statusRunning || job.Status == statusPending || job.Status == statusCreated {
			active = append(active, job)
		}
	}
	return active
}

// getFailedJobs returns jobs that have failed.
func (jt *jobTracker) getFailedJobs() []*Job {
	jt.mu.RLock()
	defer jt.mu.RUnlock()

	var failed []*Job
	for _, job := range jt.jobs {
		if job.Status == statusFailed {
			failed = append(failed, job)
		}
	}
	return failed
}

// getAllJobs returns a copy of all tracked jobs.
func (jt *jobTracker) getAllJobs() []*Job {
	jt.mu.RLock()
	defer jt.mu.RUnlock()

	jobs := make([]*Job, 0, len(jt.jobs))
	for _, job := range jt.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}