package github

import (
	"sync"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/sgaunet/bullets"
)

// Constants for GitHub API operations.
const (
	minURLParts            = 2
	maxCheckRunsPerPage    = 100
	maxJobDetailsToDisplay = 3
	checkPollInterval      = 5 * time.Second
	spinnerUpdateInterval  = 1 * time.Second
	workflowCreationDelay  = 5 * time.Second
	conclusionSuccess      = "success"
	statusInProgress       = "in_progress"
	statusQueued           = "queued"
	statusCompleted        = "completed"
	conclusionSkipped      = "skipped"
	conclusionNeutral      = "neutral"
)

// Client represents a GitHub API client wrapper that manages pull request
// lifecycle operations. It stores internal state (owner, repo, prNumber, prSHA)
// that is set by methods like [Client.SetRepositoryFromURL] and [Client.CreatePullRequest].
//
// Not safe for concurrent use.
type Client struct {
	client  *github.Client
	owner   string
	repo    string
	prNumber int
	prSHA   string
	log     *bullets.Logger
	display *displayRenderer // Display renderer for UI output
}

// Label represents a GitHub label.
type Label struct {
	Name string
}

// JobInfo represents a GitHub workflow job with detailed status information.
// Status values are: "queued", "in_progress", "completed".
// Conclusion values (only set when completed): "success", "failure", "cancelled", "skipped", "neutral".
type JobInfo struct {
	ID          int64      // Unique job ID
	Name        string     // Job name as defined in workflow YAML
	Status      string     // Current job status
	Conclusion  string     // Final conclusion (empty until completed)
	StartedAt   *time.Time // When the job started (nil if queued)
	CompletedAt *time.Time // When the job finished (nil if still running)
	HTMLURL     string     // Browser URL for the job
}

// checkTracker tracks workflow jobs/checks and their display handles with thread-safe access.
type checkTracker struct {
	mu       sync.RWMutex
	checks   map[int64]*JobInfo
	handles  map[int64]*bullets.BulletHandle
	spinners map[int64]*bullets.Spinner // Spinners for running jobs
}
