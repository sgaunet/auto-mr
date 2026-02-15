package gitlab

import (
	"sync"
	"time"

	"github.com/sgaunet/bullets"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Constants for GitLab API operations.
const (
	minURLParts            = 2
	pipelinePollInterval   = 5 * time.Second
	spinnerUpdateInterval  = 1 * time.Second
	maxJobDetailsToDisplay = 3
	statusSuccess          = "success"
	statusRunning          = "running"
	statusPending          = "pending"
	statusCreated          = "created"
	statusFailed           = "failed"
	statusCanceled         = "canceled"
	statusSkipped          = "skipped"
)

// Client represents a GitLab API client wrapper that manages merge request
// lifecycle operations. It stores internal state (projectID, mrIID, mrSHA)
// that is set by methods like [Client.SetProjectFromURL] and [Client.CreateMergeRequest].
//
// Not safe for concurrent use.
type Client struct {
	client       *gitlab.Client
	projectID    string
	mrIID        int64
	mrSHA        string
	log          *bullets.Logger
	updatableLog *bullets.UpdatableLogger
	display      *displayRenderer // Display renderer for UI output
}

// Label represents a GitLab label.
type Label struct {
	Name string
}

// Job represents a GitLab pipeline job with detailed status information.
// Status values are: "created", "pending", "running", "success", "failed", "canceled", "skipped".
type Job struct {
	ID         int64      // Unique job ID
	Name       string     // Job name as defined in .gitlab-ci.yml
	Status     string     // Current job status
	Stage      string     // Pipeline stage (e.g., "build", "test", "deploy")
	CreatedAt  time.Time  // When the job was created
	StartedAt  *time.Time // When the job started running (nil if not started)
	FinishedAt *time.Time // When the job finished (nil if still running)
	Duration   float64    // Job duration in seconds
	WebURL     string     // Browser URL for the job
}

// jobTracker tracks jobs and their display handles/spinners with thread-safe access.
type jobTracker struct {
	mu       sync.RWMutex
	jobs     map[int64]*Job
	handles  map[int64]*bullets.BulletHandle
	spinners map[int64]*bullets.Spinner
}
