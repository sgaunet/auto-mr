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
	errTokenRequired    = errors.New("GITLAB_TOKEN environment variable is required")
	errInvalidURLFormat = errors.New("invalid GitLab URL format")
	errAssigneeNotFound = errors.New("failed to find assignee user")
	errReviewerNotFound = errors.New("failed to find reviewer user")
	errPipelineTimeout  = errors.New("timeout waiting for pipeline completion")
	errMRNotFound       = errors.New("no merge request found for branch")
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
	statusCanceled = "canceled"
	statusSkipped  = "skipped"
)

// Client represents a GitLab API client wrapper.
type Client struct {
	client       *gitlab.Client
	projectID    string
	mrIID        int
	mrSHA        string
	log          *bullets.Logger
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

// jobTracker tracks jobs and their display handles/spinners with thread-safe access.
type jobTracker struct {
	mu       sync.RWMutex
	jobs     map[int]*Job
	handles  map[int]*bullets.BulletHandle
	spinners map[int]*bullets.Spinner
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
		client:       client,
		log:          logger.NoLogger(),
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
			// Wait silently for pipelines to appear (they'll show as individual spinners when they start)
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
			c.updatableLog.Success(fmt.Sprintf("Pipeline completed successfully - total time: %s",
				formatDuration(totalDuration)))
		} else {
			msg := fmt.Sprintf("Pipeline failed - total time: %s",
				formatDuration(totalDuration))
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
		Squash:                   gitlab.Ptr(squash),
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

// formatJobStatus formats a job status with duration.
// Returns a formatted string like "build (running, 1m 23s)" or "test (success, 45s)".
// Icons are added by the bullets library methods (Success/Error/etc), not by this function.
func formatJobStatus(job *Job) string {
	if job == nil {
		return ""
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

	// Format the complete status string (without icon - bullets library adds those)
	if durationStr != "" {
		return fmt.Sprintf("%s (%s, %s)", jobName, job.Status, durationStr)
	}
	return fmt.Sprintf("%s (%s)", jobName, job.Status)
}

// newJobTracker creates a new job tracker with initialized maps.
func newJobTracker() *jobTracker {
	return &jobTracker{
		jobs:     make(map[int]*Job),
		handles:  make(map[int]*bullets.BulletHandle),
		spinners: make(map[int]*bullets.Spinner),
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

// getSpinner retrieves a spinner by job ID with read lock.
func (jt *jobTracker) getSpinner(id int) (*bullets.Spinner, bool) {
	jt.mu.RLock()
	defer jt.mu.RUnlock()
	spinner, exists := jt.spinners[id]
	return spinner, exists
}

// setSpinner stores a spinner for a job ID with write lock.
func (jt *jobTracker) setSpinner(id int, spinner *bullets.Spinner) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.spinners[id] = spinner
}

// deleteSpinner stops and removes a spinner with write lock.
func (jt *jobTracker) deleteSpinner(id int) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	if spinner, exists := jt.spinners[id]; exists {
		spinner.Stop()
		delete(jt.spinners, id)
	}
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
			// New job detected
			jt.setJob(newJob.ID, newJob)
			statusText := formatJobStatus(newJob)

			if newJob.Status == statusRunning {
				// Create animated spinner for running jobs
				spinner := logger.SpinnerCircle(statusText)
				jt.setSpinner(newJob.ID, spinner)
			} else {
				// Create static handle for non-running jobs
				handle := logger.InfoHandle(statusText)
				jt.setHandle(newJob.ID, handle)
			}
			transitions = append(transitions, fmt.Sprintf("Job %d started: %s/%s", newJob.ID, newJob.Stage, newJob.Name))
		} else if oldJob.Status != newJob.Status {
			// Status changed - update display and handle state transitions
			wasPulsing := oldJob.Status == statusRunning
			isPulsing := newJob.Status == statusRunning

			jt.updateHandleForJob(logger, newJob, wasPulsing, isPulsing)
			jt.setJob(newJob.ID, newJob)
			transitions = append(transitions, fmt.Sprintf("Job %d: %s -> %s", newJob.ID, oldJob.Status, newJob.Status))
		} else {
			// No status change, just update job data (timestamps/duration may have changed)
			jt.setJob(newJob.ID, newJob)
			// Update text only for non-running jobs (spinners display automatically)
			if newJob.Status != statusRunning {
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
// wasPulsing and isPulsing control whether to start or stop the spinner animation.
func (jt *jobTracker) updateHandleForJob(logger *bullets.UpdatableLogger, job *Job, wasPulsing, isPulsing bool) {
	statusText := formatJobStatus(job)

	// Handle completed jobs - finalize spinner or handle
	if job.Status == statusSuccess || job.Status == statusFailed || job.Status == statusCanceled {
		// If was running, stop spinner with final message
		if spinner, exists := jt.getSpinner(job.ID); exists {
			switch job.Status {
			case statusSuccess:
				spinner.Success(statusText)
			case statusCanceled:
				spinner.Replace(statusText) // Use Replace for canceled (neutral outcome)
			default:
				spinner.Error(statusText)
			}
			jt.deleteSpinner(job.ID)
		} else if handle, exists := jt.getHandle(job.ID); exists {
			// Was not running, update handle
			switch job.Status {
			case statusSuccess:
				handle.Success(statusText)
			case statusCanceled:
				handle.Warning(statusText)
			default:
				handle.Error(statusText)
			}
		}
		return
	}

	// Transition from non-running to running: create spinner
	if isPulsing && !wasPulsing {
		// Stop any existing handle
		if handle, exists := jt.getHandle(job.ID); exists {
			handle.Update(bullets.InfoLevel, "") // Clear the line
			jt.mu.Lock()
			delete(jt.handles, job.ID)
			jt.mu.Unlock()
		}
		// Create animated spinner
		spinner := logger.SpinnerCircle(statusText)
		jt.setSpinner(job.ID, spinner)
		return
	}

	// Transition from running to non-running: create handle
	if !isPulsing && wasPulsing {
		// Stop spinner
		if spinner, exists := jt.getSpinner(job.ID); exists {
			spinner.Replace(statusText)
			jt.deleteSpinner(job.ID)
		}
		// Create static handle
		handle := logger.InfoHandle(statusText)
		jt.setHandle(job.ID, handle)
		return
	}

	// No animation state change - update existing display
	if _, exists := jt.getSpinner(job.ID); exists {
		// Spinner is running, no update needed (animation continues)
		// Spinner doesn't support text updates during animation
		return
	}
	if handle, exists := jt.getHandle(job.ID); exists {
		// Static handle, update text
		handle.Update(bullets.InfoLevel, statusText)
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
			// Stop and cleanup any spinners
			if spinner, exists := jt.spinners[id]; exists {
				spinner.Stop()
				delete(jt.spinners, id)
			}
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
	// Stop all spinners before clearing
	for _, spinner := range jt.spinners {
		spinner.Stop()
	}
	jt.jobs = make(map[int]*Job)
	jt.handles = make(map[int]*bullets.BulletHandle)
	jt.spinners = make(map[int]*bullets.Spinner)
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
