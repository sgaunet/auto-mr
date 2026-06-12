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
	"fmt"
	"os"
	"strings"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/internal/timeutil"
	"github.com/sgaunet/bullets"
)

// NewClient creates a new Forgejo client authenticated via the FORGEJO_TOKEN environment variable.
//
// Parameters:
//   - baseURL: the base URL of the Forgejo instance (e.g. "https://forgejo.example.com")
//
// Returns [ErrTokenRequired] if FORGEJO_TOKEN is not set.
func NewClient(baseURL string) (*Client, error) {
	token := strings.TrimSpace(os.Getenv("FORGEJO_TOKEN"))
	if token == "" {
		return nil, errTokenRequired
	}

	client, err := gitea.NewClient(baseURL, gitea.SetToken(token))
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
func (c *Client) WaitForPipeline(timeout time.Duration) (string, error) {
	c.log.Debug(fmt.Sprintf("Waiting for pipeline, SHA: %s, timeout: %v", c.prSHA, timeout))
	start := time.Now()

	c.display.Info("Waiting for pipeline to complete...")
	c.display.IncreasePadding()
	defer c.display.DecreasePadding()

	tracker := newStatusTracker()
	emptyPollCount := 0

	for time.Since(start) < timeout {
		cs, _, err := c.client.GetCombinedStatus(c.owner, c.repo, c.prSHA)
		if err != nil {
			c.display.Error(fmt.Sprintf("Failed to get combined status: %v", err))
			return "", fmt.Errorf("failed to get combined status: %w", err)
		}

		// No statuses at all – apply grace period before treating as "no CI".
		if len(cs.Statuses) == 0 {
			emptyPollCount++
			if emptyPollCount > pipelineGraceCycles {
				c.log.Info("No commit statuses configured, treating as success")
				c.display.Success("No CI configured — proceeding")
				return stateSuccess, nil
			}

			time.Sleep(statusPollInterval)
			continue
		}

		// Statuses appeared — reset grace counter.
		emptyPollCount = 0

		// Update tracker spinners/handles for each status context.
		transitions := tracker.update(cs.Statuses, c.display.GetUpdatable())
		for _, t := range transitions {
			c.log.Debug(t)
		}

		// Check aggregate result.
		result, done := aggregateResult(cs)
		if !done {
			time.Sleep(statusPollInterval)
			continue
		}

		// All statuses resolved.
		totalDuration := time.Since(start)
		if result == stateSuccess || result == stateWarning {
			c.display.Success("Pipeline completed successfully — total time: " +
				timeutil.FormatDuration(totalDuration))
			return stateSuccess, nil
		}

		msg := fmt.Sprintf("Pipeline %s — total time: %s", result, timeutil.FormatDuration(totalDuration))
		handle := c.display.InfoHandle(msg)
		handle.Error(msg)
		return result, nil
	}

	totalDuration := time.Since(start)
	c.display.Error("Timeout after " + timeutil.FormatDuration(totalDuration))
	return "", errWorkflowTimeout
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

