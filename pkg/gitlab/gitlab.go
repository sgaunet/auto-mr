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
	statusCanceled         = "canceled"
	statusSkipped          = "skipped"
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
		allCompleted, overallStatus := c.processPipelinesWithJobTracking(pipelines, tracker)

		if !allCompleted {
			time.Sleep(pipelinePollInterval)
			continue
		}

		// All pipelines completed - display final summary
		totalDuration := time.Since(start)
		if overallStatus == statusSuccess {
			c.updatableLog.Success("Pipeline completed successfully - total time: " +
				formatDuration(totalDuration))
		} else {
			msg := "Pipeline failed - total time: " +
				formatDuration(totalDuration)
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

// processPipelinesWithJobTracking evaluates all pipeline statuses using jobTracker for individual job display.
func (c *Client) processPipelinesWithJobTracking(
	pipelines []*gitlab.PipelineInfo, tracker *jobTracker,
) (bool, string) {
	// Fetch jobs for all pipelines in parallel
	allJobs, failedPipelines := c.fetchJobsForPipelines(pipelines)

	// If no jobs found, fall back to pipeline-level view with individual spinners
	if len(allJobs) == 0 {
		return c.processPipelinesFallback(tracker, pipelines)
	}

	// If some pipelines failed to fetch jobs, convert them to pseudo-jobs for display
	if len(failedPipelines) > 0 {
		fallbackJobs := c.convertPipelinesToJobs(failedPipelines)
		allJobs = append(allJobs, fallbackJobs...)
	}

	// Update job tracker with new jobs (creates/updates handles automatically)
	transitions := tracker.update(allJobs, c.updatableLog)
	for _, transition := range transitions {
		c.log.Debug(transition)
	}

	// Analyze job statuses for completion
	return c.analyzePipelineJobCompletion(allJobs)
}

// fetchJobsForPipelines fetches jobs for multiple pipelines concurrently.
func (c *Client) fetchJobsForPipelines(pipelines []*gitlab.PipelineInfo) ([]*Job, []*gitlab.PipelineInfo) {
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
	var failedPipelines []*gitlab.PipelineInfo

	for result := range resultChan {
		if result.err != nil {
			c.log.Debug(fmt.Sprintf("Failed to fetch jobs for pipeline %d: %v", result.pipelineID, result.err))
			// Track failed pipelines for fallback processing
			for _, p := range pipelines {
				if p.ID == result.pipelineID {
					failedPipelines = append(failedPipelines, p)
					break
				}
			}
			continue
		}
		allJobs = append(allJobs, result.jobs...)
	}

	return allJobs, failedPipelines
}

// analyzePipelineJobCompletion checks if all jobs are completed and determines overall status.
func (c *Client) analyzePipelineJobCompletion(allJobs []*Job) (bool, string) {
	allCompleted := true
	overallStatus := statusSuccess

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

// processPipelinesFallback processes pipelines using jobTracker for individual spinners.
// This is used as a fallback when job-level APIs are unavailable.
func (c *Client) processPipelinesFallback(tracker *jobTracker, pipelines []*gitlab.PipelineInfo) (bool, string) {
	// Convert pipelines to Job format for tracker
	jobs := c.convertPipelinesToJobs(pipelines)

	// Update job tracker with converted jobs (creates/updates spinners automatically)
	transitions := tracker.update(jobs, c.updatableLog)
	for _, transition := range transitions {
		c.log.Debug(transition)
	}

	// Analyze completion status
	allCompleted := true
	overallStatus := statusSuccess

	for _, job := range jobs {
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

// convertPipelinesToJobs converts pipelines to Job format for display with jobTracker.
func (c *Client) convertPipelinesToJobs(pipelines []*gitlab.PipelineInfo) []*Job {
	jobs := make([]*Job, 0, len(pipelines))

	for _, pipeline := range pipelines {
		if pipeline == nil {
			continue
		}

		// Create a pseudo-job representing the pipeline
		job := &Job{
			ID:     pipeline.ID,
			Name:   fmt.Sprintf("Pipeline #%d", pipeline.ID),
			Stage:  pipeline.Ref, // Use ref as stage for context
			Status: pipeline.Status,
		}

		// Set timestamps if available
		if pipeline.CreatedAt != nil {
			job.StartedAt = pipeline.CreatedAt
		}
		if pipeline.UpdatedAt != nil {
			job.FinishedAt = pipeline.UpdatedAt
		}

		// Calculate duration from timestamps if both available
		if job.StartedAt != nil && job.FinishedAt != nil {
			duration := job.FinishedAt.Sub(*job.StartedAt)
			job.Duration = duration.Seconds()
		}

		jobs = append(jobs, job)
	}

	return jobs
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

// update processes new jobs, detects state transitions, and updates handles.
// Returns a list of state transition descriptions.
func (jt *jobTracker) update(newJobs []*Job, logger *bullets.UpdatableLogger) []string {
	var transitions []string
	newJobIDs := make(map[int]bool)

	for _, newJob := range newJobs {
		if newJob == nil || newJob.ID == 0 || newJobIDs[newJob.ID] {
			continue
		}

		newJobIDs[newJob.ID] = true
		transition := jt.processJobUpdate(newJob, logger)
		if transition != "" {
			transitions = append(transitions, transition)
		}
	}

	// Detect removed jobs
	transitions = append(transitions, jt.detectRemovedJobs(newJobIDs)...)

	return transitions
}

// processJobUpdate handles the update logic for a single job.
func (jt *jobTracker) processJobUpdate(newJob *Job, logger *bullets.UpdatableLogger) string {
	oldJob, exists := jt.getJob(newJob.ID)

	switch {
	case !exists:
		return jt.handleNewJob(newJob, logger)
	case oldJob.Status != newJob.Status:
		return jt.handleJobStatusChange(oldJob, newJob, logger)
	default:
		return jt.handleJobDataUpdate(newJob)
	}
}

// handleNewJob processes a newly detected job.
func (jt *jobTracker) handleNewJob(newJob *Job, logger *bullets.UpdatableLogger) string {
	jt.setJob(newJob.ID, newJob)
	statusText := formatJobStatus(newJob)

	if newJob.Status == statusRunning {
		spinner := logger.SpinnerCircle(statusText)
		jt.setSpinner(newJob.ID, spinner)
	} else {
		handle := logger.InfoHandle(statusText)
		jt.setHandle(newJob.ID, handle)
	}

	return fmt.Sprintf("Job %d started: %s/%s", newJob.ID, newJob.Stage, newJob.Name)
}

// handleJobStatusChange processes a job with changed status.
func (jt *jobTracker) handleJobStatusChange(oldJob, newJob *Job, logger *bullets.UpdatableLogger) string {
	wasPulsing := oldJob.Status == statusRunning
	isPulsing := newJob.Status == statusRunning

	jt.updateHandleForJob(logger, newJob, wasPulsing, isPulsing)
	jt.setJob(newJob.ID, newJob)
	return fmt.Sprintf("Job %d: %s -> %s", newJob.ID, oldJob.Status, newJob.Status)
}

// handleJobDataUpdate updates job data without status change.
func (jt *jobTracker) handleJobDataUpdate(newJob *Job) string {
	jt.setJob(newJob.ID, newJob)
	// Update text only for non-running jobs (spinners display automatically)
	if newJob.Status != statusRunning {
		if handle, exists := jt.getHandle(newJob.ID); exists {
			statusText := formatJobStatus(newJob)
			handle.Update(bullets.InfoLevel, statusText)
		}
	}
	return ""
}

// detectRemovedJobs detects jobs that have been removed.
func (jt *jobTracker) detectRemovedJobs(newJobIDs map[int]bool) []string {
	var transitions []string
	jt.mu.RLock()
	defer jt.mu.RUnlock()

	for id := range jt.jobs {
		if !newJobIDs[id] {
			transitions = append(transitions, fmt.Sprintf("Job %d removed", id))
		}
	}
	return transitions
}

// updateHandleForJob updates the display for a job when status changes.
// wasPulsing and isPulsing control whether to start or stop the spinner animation.
func (jt *jobTracker) updateHandleForJob(logger *bullets.UpdatableLogger, job *Job, wasPulsing, isPulsing bool) {
	statusText := formatJobStatus(job)

	if job.Status == statusSuccess || job.Status == statusFailed || job.Status == statusCanceled {
		jt.finalizeCompletedJob(job, statusText)
		return
	}

	if isPulsing && !wasPulsing {
		jt.transitionJobToRunning(logger, job.ID, statusText)
		return
	}

	if !isPulsing && wasPulsing {
		jt.transitionJobToNonRunning(logger, job.ID, statusText)
		return
	}

	jt.updateExistingJobDisplay(job.ID, statusText)
}

// finalizeCompletedJob handles completed jobs - finalize spinner or handle.
func (jt *jobTracker) finalizeCompletedJob(job *Job, statusText string) {
	// If was running, stop spinner with final message
	if spinner, exists := jt.getSpinner(job.ID); exists {
		jt.finalizeJobSpinner(spinner, job.Status, statusText)
		jt.deleteSpinner(job.ID)
		return
	}

	// Was not running, update handle
	if handle, exists := jt.getHandle(job.ID); exists {
		jt.finalizeJobHandle(handle, job.Status, statusText)
	}
}

// finalizeJobSpinner stops a spinner with the appropriate final message.
func (jt *jobTracker) finalizeJobSpinner(spinner *bullets.Spinner, status, statusText string) {
	switch status {
	case statusSuccess:
		spinner.Success(statusText)
	case statusCanceled:
		spinner.Replace(statusText) // Use Replace for canceled (neutral outcome)
	default:
		spinner.Error(statusText)
	}
}

// finalizeJobHandle updates a handle with the appropriate final status.
func (jt *jobTracker) finalizeJobHandle(handle *bullets.BulletHandle, status, statusText string) {
	switch status {
	case statusSuccess:
		handle.Success(statusText)
	case statusCanceled:
		handle.Warning(statusText)
	default:
		handle.Error(statusText)
	}
}

// transitionJobToRunning creates a spinner when a job transitions to running state.
func (jt *jobTracker) transitionJobToRunning(logger *bullets.UpdatableLogger, jobID int, statusText string) {
	// Stop any existing handle
	if handle, exists := jt.getHandle(jobID); exists {
		handle.Update(bullets.InfoLevel, "") // Clear the line
		jt.mu.Lock()
		delete(jt.handles, jobID)
		jt.mu.Unlock()
	}
	// Create animated spinner
	spinner := logger.SpinnerCircle(statusText)
	jt.setSpinner(jobID, spinner)
}

// transitionJobToNonRunning creates a handle when a job transitions from running state.
func (jt *jobTracker) transitionJobToNonRunning(logger *bullets.UpdatableLogger, jobID int, statusText string) {
	// Stop spinner
	if spinner, exists := jt.getSpinner(jobID); exists {
		spinner.Replace(statusText)
		jt.deleteSpinner(jobID)
	}
	// Create static handle
	handle := logger.InfoHandle(statusText)
	jt.setHandle(jobID, handle)
}

// updateExistingJobDisplay updates existing display without animation state change.
func (jt *jobTracker) updateExistingJobDisplay(jobID int, statusText string) {
	if _, exists := jt.getSpinner(jobID); exists {
		// Spinner is running, no update needed (animation continues)
		return
	}
	if handle, exists := jt.getHandle(jobID); exists {
		// Static handle, update text
		handle.Update(bullets.InfoLevel, statusText)
	}
}
