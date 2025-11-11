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
	display      *displayRenderer // Display renderer for UI output
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
