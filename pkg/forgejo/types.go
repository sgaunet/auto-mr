package forgejo

import (
	"sync"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/sgaunet/bullets"
)

// Constants for Forgejo API operations.
const (
	minURLParts         = 2
	statusPollInterval  = 5 * time.Second
	spinnerUpdateInterval = 1 * time.Second
	pipelineGraceCycles = 2 // grace poll cycles before treating "no statuses" as success
)

// State string constants for CI status display.
const (
	stateSuccess = "success"
	stateFailure = "failure"
	stateError   = "error"
	statePending = "pending"
	stateWarning = "warning"
)

// Client represents a Forgejo API client wrapper that manages pull request
// lifecycle operations. It stores internal state (owner, repo, prIndex, prSHA)
// that is set by methods like [Client.SetRepositoryFromURL] and [Client.CreatePullRequest].
//
// Not safe for concurrent use.
type Client struct {
	client      *gitea.Client
	owner       string
	repo        string
	prIndex     int64
	prSHA       string
	log         *bullets.Logger
	updatableLog *bullets.UpdatableLogger
	display     *displayRenderer
}

// Label represents a Forgejo repository label.
type Label struct {
	Name string
}

// statusEntry holds the per-status-context display state used by [statusTracker].
type statusEntry struct {
	context     string
	state       gitea.StatusState
	targetURL   string
	description string
}

// statusTracker tracks commit-status entries and their display handles.
// Keyed by context string (e.g. "ci/test", "ci/build").
type statusTracker struct {
	mu       sync.RWMutex
	entries  map[string]*statusEntry
	handles  map[string]*bullets.BulletHandle
	spinners map[string]*bullets.Spinner
}
