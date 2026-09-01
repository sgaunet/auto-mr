package gitlab

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/internal/polling"
	"github.com/sgaunet/auto-mr/internal/timeutil"
	"github.com/sgaunet/auto-mr/internal/urlutil"
	"github.com/sgaunet/bullets"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Option configures [NewClient].
type Option func(*clientOptions)

// clientOptions holds settings supplied to [NewClient] via [Option] values.
type clientOptions struct {
	baseURL string
}

// WithBaseURL points the client at a specific GitLab API endpoint instead of
// gitlab.com.
//
// The SDK accepts a base URL only at construction and exposes no setter, so this is
// the sole way to redirect the client — which is what lets tests exercise the real
// client against an httptest server rather than a fake that echoes its own inputs.
func WithBaseURL(url string) Option {
	return func(o *clientOptions) { o.baseURL = url }
}

// NewClient creates a new GitLab client authenticated via the GITLAB_TOKEN environment variable.
//
// Returns [ErrTokenRequired] if GITLAB_TOKEN is not set.
// Returns a wrapped error if the underlying GitLab client creation fails.
func NewClient(opts ...Option) (*Client, error) {
	token := strings.TrimSpace(os.Getenv("GITLAB_TOKEN"))
	if token == "" {
		return nil, errTokenRequired
	}

	var options clientOptions
	for _, opt := range opts {
		opt(&options)
	}

	var sdkOpts []gitlab.ClientOptionFunc
	if options.baseURL != "" {
		sdkOpts = append(sdkOpts, gitlab.WithBaseURL(options.baseURL))
	}

	client, err := gitlab.NewClient(token, sdkOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	log := logger.NoLogger()
	updatable := bullets.NewUpdatable(os.Stdout)

	return &Client{
		client:       client,
		log:          log,
		updatableLog: updatable,
		display:      newDisplayRenderer(log, updatable),
	}, nil
}

// SetLogger sets the logger for the GitLab client.
func (c *Client) SetLogger(logger *bullets.Logger) {
	c.log = logger
	c.updatableLog.Logger = logger
	c.display.SetLogger(logger)
	c.log.Debug("GitLab client logger configured")
}

// SetProjectFromURL sets the project from a git remote URL.
// Supports both HTTPS and SSH URL formats:
//   - https://gitlab.com/group/project.git
//   - git@gitlab.com:group/project.git
//
// The .git suffix should already be present; it is stripped internally.
//
// Returns [ErrInvalidURLFormat] if the URL cannot be parsed.
// Returns a wrapped error if the project does not exist or the API call fails.
func (c *Client) SetProjectFromURL(ctx context.Context, url string) error {
	// Bound the operation so a stalled response cannot hang a caller whose own
	// context carries no deadline.
	ctx, cancel := context.WithTimeout(ctx, polling.PerCallTimeout)
	defer cancel()

	// Extract project path from URL
	// Supports both HTTPS and SSH formats:
	// - https://gitlab.com/user/project.git
	// - git@gitlab.com:user/project.git
	url = strings.TrimSuffix(url, ".git")

	projectPath := urlutil.ExtractPathComponents(url, minURLParts)
	if projectPath == "" {
		return errInvalidURLFormat
	}

	c.log.Debug("Setting GitLab project: " + projectPath)

	// Get project info to validate and get project ID
	project, _, err := c.client.Projects.GetProject(projectPath, nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to get project information: %w", err)
	}

	c.projectID = strconv.FormatInt(project.ID, 10)
	c.log.Debug("GitLab project set, ID: " + c.projectID)
	return nil
}

// ListLabels returns all labels for the project.
// [SetProjectFromURL] must be called before this method.
//
// Returns an empty slice if no labels are configured.
func (c *Client) ListLabels(ctx context.Context) ([]*Label, error) {
	// Bound the operation so a stalled response cannot hang a caller whose own
	// context carries no deadline.
	ctx, cancel := context.WithTimeout(ctx, polling.PerCallTimeout)
	defer cancel()

	c.log.Debug("Listing GitLab labels")

	labels, _, err := c.client.Labels.ListLabels(c.projectID, &gitlab.ListLabelsOptions{
		IncludeAncestorGroups: new(true),
	}, gitlab.WithContext(ctx))
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
// The created MR automatically sets RemoveSourceBranch to true.
//
// Parameters:
//   - sourceBranch: the feature branch name
//   - targetBranch: the target branch (e.g., "main")
//   - title: MR title (must not be empty)
//   - description: MR body/description
//   - assignee: GitLab username to assign
//   - reviewer: GitLab username to request review from
//   - labels: list of label names to apply (may be nil)
//   - squash: whether to squash commits on merge
//
// Returns [ErrMRAlreadyExists] if an MR already exists for the same branches.
// Returns [ErrAssigneeNotFound] or [ErrReviewerNotFound] if users cannot be found.
// Stores the MR IID and SHA internally for use by [Client.WaitForPipeline].
func (c *Client) CreateMergeRequest(
	ctx context.Context,
	sourceBranch, targetBranch, title, description, assignee, reviewer string,
	labels []string, squash bool,
) (*gitlab.MergeRequest, error) {
	// Bound the operation so a stalled response cannot hang a caller whose own
	// context carries no deadline.
	ctx, cancel := context.WithTimeout(ctx, polling.OperationTimeout)
	defer cancel()

	c.log.Debug(fmt.Sprintf("Creating merge request from %s to %s", sourceBranch, targetBranch))

	// Get user IDs for assignee and reviewer
	assigneeUser, _, err := c.client.Users.ListUsers(&gitlab.ListUsersOptions{
		Username: &assignee,
	}, gitlab.WithContext(ctx))
	if err != nil || len(assigneeUser) == 0 {
		return nil, fmt.Errorf("%w: %s", errAssigneeNotFound, assignee)
	}

	reviewerUser, _, err := c.client.Users.ListUsers(&gitlab.ListUsersOptions{
		Username: &reviewer,
	}, gitlab.WithContext(ctx))
	if err != nil || len(reviewerUser) == 0 {
		return nil, fmt.Errorf("%w: %s", errReviewerNotFound, reviewer)
	}

	assigneeID := assigneeUser[0].ID
	reviewerIDs := []int64{reviewerUser[0].ID}

	labelOptions := (*gitlab.LabelOptions)(&labels)
	createOptions := &gitlab.CreateMergeRequestOptions{
		Title:              &title,
		Description:        &description,
		SourceBranch:       &sourceBranch,
		TargetBranch:       &targetBranch,
		AssigneeID:         &assigneeID,
		ReviewerIDs:        &reviewerIDs,
		Labels:             labelOptions,
		Squash:             new(squash),
		RemoveSourceBranch: new(true),
	}

	mr, _, err := c.client.MergeRequests.CreateMergeRequest(c.projectID, createOptions, gitlab.WithContext(ctx))
	if err != nil {
		// Check if error indicates MR already exists
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "already exists") ||
			strings.Contains(errMsg, "another open merge request already exists") {
			return nil, fmt.Errorf("%w: source=%s, target=%s: %w",
				errMRAlreadyExists, sourceBranch, targetBranch, err)
		}
		return nil, fmt.Errorf("failed to create merge request: %w", err)
	}

	c.mrIID = mr.IID
	c.mrSHA = mr.SHA
	c.log.Debug(fmt.Sprintf("Merge request created - IID: %d, SHA: %s, URL: %s", mr.IID, mr.SHA, mr.WebURL))
	return mr, nil
}

// GetMergeRequestByBranch fetches an existing open merge request by source and target branches.
// Only the first matching MR is returned. Stores the MR IID and SHA internally.
//
// Returns [ErrMRNotFound] if no open MR matches the given branches.
func (c *Client) GetMergeRequestByBranch(
	ctx context.Context, sourceBranch, targetBranch string,
) (*gitlab.MergeRequest, error) {
	// Bound the operation so a stalled response cannot hang a caller whose own
	// context carries no deadline.
	ctx, cancel := context.WithTimeout(ctx, polling.PerCallTimeout)
	defer cancel()

	mrs, _, err := c.client.MergeRequests.ListProjectMergeRequests(c.projectID, &gitlab.ListProjectMergeRequestsOptions{
		State:        new("opened"),
		SourceBranch: &sourceBranch,
		TargetBranch: &targetBranch,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list merge requests: %w", err)
	}

	if len(mrs) == 0 {
		return nil, fmt.Errorf("%w: %s", errMRNotFound, sourceBranch)
	}

	// Get full MR details
	mr, _, err := c.client.MergeRequests.GetMergeRequest(c.projectID, mrs[0].IID, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request details: %w", err)
	}

	c.mrIID = mr.IID
	c.mrSHA = mr.SHA
	return mr, nil
}

// WaitForPipeline waits for all pipelines to complete for the merge request.
// It polls at 5-second intervals and displays real-time job-level progress with animated spinners.
// If no pipelines are configured, it returns "success" immediately.
//
// Parameters:
//   - timeout: maximum wait duration (typically 1m to 8h)
//
// Returns the overall pipeline status ("success", "failed", "canceled").
// Returns [ErrPipelineTimeout] if the timeout is exceeded.
//
// A merge request must have been created or fetched before calling this method.
func (c *Client) WaitForPipeline(ctx context.Context, timeout time.Duration) (string, error) {
	c.log.Debug(fmt.Sprintf("Waiting for pipeline, timeout: %v", timeout))
	start := time.Now()

	// The overall budget bounds the whole wait; each individual request additionally
	// gets a short deadline of its own, so one stalled response cannot consume the
	// entire budget. Only overallCtx can distinguish "this request stalled, poll
	// again" from "the budget is spent": a per-request context is derived from it and
	// so reports an error in both cases.
	overallCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// First check if any pipelines are expected for this commit
	if !c.hasPipelineRuns(overallCtx) {
		c.log.Info("No pipeline runs configured for this merge request, proceeding without checks")
		return statusSuccess, nil
	}

	// Create updatable handle for pipeline status
	c.updatableLog.Info("Waiting for pipelines to complete...")
	c.updatableLog.IncreasePadding()
	defer c.updatableLog.DecreasePadding()

	// Initialize job tracker for managing individual job handles
	tracker := newJobTracker(overallCtx)
	defer tracker.Stop()

	for overallCtx.Err() == nil {
		status, done, err := c.pollPipelineOnce(overallCtx, tracker)
		if err != nil {
			if overallCtx.Err() != nil {
				break // Budget spent or cancelled; reported after the loop.
			}
			// No retry classification here: the GitLab SDK already retries 429 and 5xx
			// responses internally and honours Ratelimit-Reset, so an error reaching
			// this point has already survived those attempts.
			return "", err
		}
		if done {
			c.reportPipelineOutcome(status, time.Since(start))
			return status, nil
		}
		if !polling.Sleep(overallCtx, polling.DefaultSchedule.IntervalFor(time.Since(start))) {
			break
		}
	}

	totalDuration := time.Since(start)
	if errors.Is(overallCtx.Err(), context.Canceled) {
		c.updatableLog.Error("Cancelled after " + timeutil.FormatDuration(totalDuration))
		return "", errPipelineCanceled
	}
	c.updatableLog.Error("Timeout after " + timeutil.FormatDuration(totalDuration))
	return "", errPipelineTimeout
}

// pollPipelineOnce performs a single polling round, bounded by its own short
// deadline so a stalled request cannot consume the overall budget.
//
// It reports the pipeline status and whether the pipelines have all reached a
// terminal state. An error accompanied by a finished overallCtx means the wait
// itself ended rather than the request failing, and the caller distinguishes those.
func (c *Client) pollPipelineOnce(
	overallCtx context.Context, tracker *jobTracker,
) (string, bool, error) {
	pollCtx, cancelPoll := context.WithTimeout(overallCtx, polling.PerCallTimeout)
	defer cancelPoll()

	pipelines, _, err := c.client.MergeRequests.ListMergeRequestPipelines(
		c.projectID, c.mrIID, nil, gitlab.WithContext(pollCtx))
	if err != nil {
		if overallCtx.Err() == nil {
			c.updatableLog.Error(fmt.Sprintf("Failed to list MR pipelines: %v", err))
		}
		return "", false, fmt.Errorf("failed to list MR pipelines: %w", err)
	}

	// Wait silently for pipelines to appear; they show as individual spinners once
	// they start.
	if len(pipelines) == 0 {
		return "", false, nil
	}

	allCompleted, overallStatus := c.processPipelinesWithJobTracking(pollCtx, pipelines, tracker)
	return overallStatus, allCompleted, nil
}

// reportPipelineOutcome renders the final summary line for a finished pipeline.
func (c *Client) reportPipelineOutcome(status string, elapsed time.Duration) {
	if status == statusSuccess {
		c.updatableLog.Success("Pipeline completed successfully - total time: " +
			timeutil.FormatDuration(elapsed))
		return
	}

	msg := "Pipeline failed - total time: " + timeutil.FormatDuration(elapsed)
	handle := c.updatableLog.InfoHandle(msg)
	handle.Error(msg)
}

// ApproveMergeRequest approves a merge request by its internal ID.
//
// Parameters:
//   - mrIID: the merge request internal ID (IID), not the global ID
func (c *Client) ApproveMergeRequest(ctx context.Context, mrIID int64) error {
	// Bound the operation so a stalled response cannot hang a caller whose own
	// context carries no deadline.
	ctx, cancel := context.WithTimeout(ctx, polling.PerCallTimeout)
	defer cancel()

	c.log.Debug(fmt.Sprintf("Approving merge request, IID: %d", mrIID))

	_, _, err := c.client.MergeRequestApprovals.ApproveMergeRequest(c.projectID, mrIID, nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to approve merge request: %w", err)
	}
	c.log.Debug("Merge request approved")
	return nil
}

// MergeMergeRequest merges a merge request with optional squash.
// The source branch is automatically removed after merge.
//
// Parameters:
//   - mrIID: the merge request internal ID
//   - squash: if true, commits are squashed and commitTitle is used as squash commit message
//   - commitTitle: the merge/squash commit message
func (c *Client) MergeMergeRequest(ctx context.Context, mrIID int64, squash bool, commitTitle string) error {
	// Bound the operation so a stalled response cannot hang a caller whose own
	// context carries no deadline.
	ctx, cancel := context.WithTimeout(ctx, polling.OperationTimeout)
	defer cancel()

	c.log.Debug(fmt.Sprintf("Merging merge request, IID: %d", mrIID))

	mergeOptions := &gitlab.AcceptMergeRequestOptions{
		Squash:                   new(squash),
		ShouldRemoveSourceBranch: new(true),
	}

	// Set commit message based on squash mode
	if squash {
		mergeOptions.SquashCommitMessage = new(commitTitle)
	} else {
		mergeOptions.MergeCommitMessage = new(commitTitle)
	}

	_, _, err := c.client.MergeRequests.AcceptMergeRequest(c.projectID, mrIID, mergeOptions, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to merge MR: %w", err)
	}

	c.log.Debug("Merge request merged successfully")
	return nil
}

// GetMergeRequestsByBranch returns all open merge requests for the given source branch.
func (c *Client) GetMergeRequestsByBranch(
	ctx context.Context, sourceBranch string,
) ([]*gitlab.BasicMergeRequest, error) {
	// Bound the operation so a stalled response cannot hang a caller whose own
	// context carries no deadline.
	ctx, cancel := context.WithTimeout(ctx, polling.PerCallTimeout)
	defer cancel()

	mrs, _, err := c.client.MergeRequests.ListProjectMergeRequests(c.projectID, &gitlab.ListProjectMergeRequestsOptions{
		SourceBranch: &sourceBranch,
		State:        new("opened"),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list merge requests: %w", err)
	}

	return mrs, nil
}

// processPipelinesWithJobTracking evaluates all pipeline statuses using jobTracker for individual job display.
func (c *Client) processPipelinesWithJobTracking(
	ctx context.Context, pipelines []*gitlab.PipelineInfo, tracker *jobTracker,
) (bool, string) {
	// Fetch jobs for all pipelines in parallel
	allJobs, failedPipelines := c.fetchJobsForPipelines(ctx, pipelines)

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
func (c *Client) fetchJobsForPipelines(
	ctx context.Context, pipelines []*gitlab.PipelineInfo,
) ([]*Job, []*gitlab.PipelineInfo) {
	type pipelineJobs struct {
		pipelineID int64
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
			jobs, err := c.fetchPipelineJobs(ctx, p.ID)
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
func (c *Client) hasPipelineRuns(ctx context.Context) bool {
	// Check for pipelines associated with this commit SHA
	pipelines, _, err := c.client.Pipelines.ListProjectPipelines(
		c.projectID,
		&gitlab.ListProjectPipelinesOptions{
			SHA: new(c.mrSHA),
		},
		gitlab.WithContext(ctx),
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
func (c *Client) fetchPipelineJobs(ctx context.Context, pipelineID int64) ([]*Job, error) {
	c.log.Debug(fmt.Sprintf("Fetching jobs for pipeline %d", pipelineID))

	var allJobs []*Job
	var page int64 = 1
	var perPage int64 = 100

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
			gitlab.WithContext(ctx),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list pipeline jobs: %w", err)
		}

		// Convert GitLab jobs to our Job struct
		for _, glJob := range jobs {
			// created_at is optional in the API response; dereferencing it blindly
			// crashes the run on a job that has not been created yet.
			var createdAt time.Time
			if glJob.CreatedAt != nil {
				createdAt = *glJob.CreatedAt
			}

			job := &Job{
				ID:         glJob.ID,
				Name:       glJob.Name,
				Status:     glJob.Status,
				Stage:      glJob.Stage,
				CreatedAt:  createdAt,
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
