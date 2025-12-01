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

// Client represents a GitHub API client wrapper.
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
	mu       sync.RWMutex
	checks   map[int64]*JobInfo
	handles  map[int64]*bullets.BulletHandle
	spinners map[int64]*bullets.Spinner // Spinners for running jobs
}
