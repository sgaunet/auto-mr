// Package forgejo provides a Forgejo API client for pull request lifecycle management.
//
// The package handles:
//   - Creating and fetching pull requests with assignees, reviewers, and labels
//   - Waiting for Forgejo Actions / commit-status CI completion with real-time visualization
//   - Merging pull requests (merge or squash strategies, with automatic branch deletion)
//   - Label retrieval for interactive selection
//
// Authentication requires a FORGEJO_TOKEN environment variable containing a
// personal access token with the required repository scopes.
//
// Usage:
//
//	client, err := forgejo.NewClient("https://forgejo.example.com")
//	client.SetLogger(logger)
//	client.SetRepositoryFromURL("https://forgejo.example.com/owner/repo.git")
//	labels, _ := client.ListLabels()
//	pr, _ := client.CreatePullRequest("feature", "main", "Title", "Body", "assignee", "reviewer", nil)
//
// Thread Safety: [Client] is not safe for concurrent use. The pipeline waiting
// methods use internal goroutines but the Client itself should be used from
// a single goroutine.
package forgejo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/internal/polling"
	"github.com/sgaunet/auto-mr/internal/timeutil"
	"github.com/sgaunet/bullets"
)

// NewClient creates a new Forgejo client authenticated via the FORGEJO_TOKEN environment variable.
//
// Parameters:
//   - baseURL: the base URL of the Forgejo instance (e.g. "https://forgejo.example.com")
//
// Returns [ErrTokenRequired] if FORGEJO_TOKEN is not set.
func NewClient(ctx context.Context, baseURL string) (*Client, error) {
	token := strings.TrimSpace(os.Getenv("FORGEJO_TOKEN"))
	if token == "" {
		return nil, errTokenRequired
	}

	// The gitea SDK defaults to &http.Client{}, which has no timeout, and exposes no
	// per-call context. An explicit HTTP timeout is therefore the only bound on an
	// individual request: without it a stalled response hangs the CLI regardless of
	// the configured pipeline budget.
	// gitea.NewClient probes the server's version over HTTP before returning, so it
	// needs a context of its own.
	client, err := gitea.NewClient(baseURL,
		gitea.SetToken(token),
		gitea.SetContext(ctx),
		gitea.SetHTTPClient(&http.Client{Timeout: polling.PerCallTimeout}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Forgejo client: %w", err)
	}

	log := logger.NoLogger()
	updatable := bullets.NewUpdatable(os.Stdout)
	display := newDisplayRenderer(log, updatable)

	return &Client{
		client:       client,
		log:          log,
		updatableLog: updatable,
		display:      display,
	}, nil
}

// SetLogger sets the logger for the Forgejo client.
func (c *Client) SetLogger(logger *bullets.Logger) {
	c.log = logger
	c.display.SetLogger(logger)
	c.log.Debug("Forgejo client logger configured")
}

// WaitForPipeline waits for all commit statuses to complete for the pull request SHA.
// It polls at 5-second intervals and displays real-time per-context progress with
// animated spinners.
//
// If no commit statuses are configured after a brief grace period, it returns "success"
// immediately (treating "no CI" as success, exactly like a repo with no workflows).
//
// Parameters:
//   - timeout: maximum wait duration (typically 1m to 8h)
//
// Returns the overall result ("success", "failure", or "error").
// Returns [ErrWorkflowTimeout] if the timeout is exceeded.
//
// A pull request must have been created or fetched before calling this method.
func (c *Client) WaitForPipeline(ctx context.Context, timeout time.Duration) (string, error) {
	c.log.Debug(fmt.Sprintf("Waiting for pipeline, SHA: %s, timeout: %v", c.prSHA, timeout))
	start := time.Now()

	// The gitea SDK exposes no per-call context, so the overall budget is the only
	// context-based bound available here; individual requests are additionally capped
	// by the HTTP client timeout configured in NewClient.
	overallCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.display.Info("Waiting for pipeline to complete...")
	c.display.IncreasePadding()
	defer c.display.DecreasePadding()

	tracker := newStatusTracker(overallCtx)
	defer tracker.Stop()
	emptyPollCount := 0

	for overallCtx.Err() == nil {
		result, done, err := c.pollStatusOnce(overallCtx, tracker, &emptyPollCount)
		if err != nil {
			if overallCtx.Err() != nil {
				break // Budget spent or cancelled; reported after the loop.
			}
			return "", err
		}
		if done {
			return c.reportPipelineOutcome(result, time.Since(start)), nil
		}
		if !polling.Sleep(overallCtx, statusPollInterval) {
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

// pollStatusOnce performs a single polling round over the commit statuses.
//
// It reports the aggregate result and whether every status has resolved. Forgejo
// repositories without CI never report any status at all, so an empty response is
// tolerated for a few cycles via emptyPollCount before being treated as "no CI
// configured" rather than as a pipeline still starting up.
func (c *Client) pollStatusOnce(
	overallCtx context.Context, tracker *statusTracker, emptyPollCount *int,
) (string, bool, error) {
	// The gitea SDK takes no per-call context, only a client-wide one. It is set
	// afresh on every call so a request never inherits a deadline belonging to an
	// earlier, unrelated operation.
	c.client.SetContext(overallCtx)

	cs, _, err := c.client.GetCombinedStatus(c.owner, c.repo, c.prSHA)
	if err != nil {
		if overallCtx.Err() == nil {
			c.display.Error(fmt.Sprintf("Failed to get combined status: %v", err))
		}
		return "", false, fmt.Errorf("failed to get combined status: %w", err)
	}

	// No statuses at all - apply grace period before treating as "no CI".
	if len(cs.Statuses) == 0 {
		*emptyPollCount++
		if *emptyPollCount > pipelineGraceCycles {
			c.log.Info("No commit statuses configured, treating as success")
			c.display.Success("No CI configured — proceeding")
			return stateSuccess, true, nil
		}
		return "", false, nil
	}

	// Statuses appeared — reset grace counter.
	*emptyPollCount = 0

	// Update tracker spinners/handles for each status context.
	transitions := tracker.update(cs.Statuses, c.display.GetUpdatable())
	for _, t := range transitions {
		c.log.Debug(t)
	}

	result, done := aggregateResult(cs)
	return result, done, nil
}

// reportPipelineOutcome renders the final summary line and returns the status to
// surface to the caller.
func (c *Client) reportPipelineOutcome(result string, elapsed time.Duration) string {
	if result == stateSuccess || result == stateWarning {
		c.display.Success("Pipeline completed successfully — total time: " +
			timeutil.FormatDuration(elapsed))
		return stateSuccess
	}

	msg := fmt.Sprintf("Pipeline %s — total time: %s", result, timeutil.FormatDuration(elapsed))
	handle := c.display.InfoHandle(msg)
	handle.Error(msg)
	return result
}

// aggregateResult determines the overall result from a CombinedStatus.
// Returns (result, done): done is false while any status is still pending.
func aggregateResult(cs *gitea.CombinedStatus) (string, bool) {
	for _, s := range cs.Statuses {
		if s == nil {
			continue
		}

		if s.State == gitea.StatusPending {
			return statePending, false
		}
	}

	// All statuses are resolved — map CombinedStatus.State.
	switch cs.State {
	case gitea.StatusSuccess:
		return stateSuccess, true
	case gitea.StatusWarning:
		return stateWarning, true
	case gitea.StatusFailure:
		return stateFailure, true
	case gitea.StatusError:
		return stateError, true
	case gitea.StatusPending:
		// Should not occur (we checked all individual statuses above), but guard it.
		return statePending, false
	default:
		// Unknown state — treat as success to avoid blocking the workflow.
		return stateSuccess, true
	}
}
