// Package git provides git repository operations for auto-mr.
//
// The package uses a hybrid approach:
//   - Push operations use go-git/go-git/v5 for proper authentication handling with tokens and SSH keys
//   - Cleanup operations (switch, pull, fetch, delete) use native git commands via exec.Command
//     to match shell script behavior and prevent silent data loss
//
// Authentication is determined automatically from the remote URL:
//   - HTTPS URLs: uses GITLAB_TOKEN or GITHUB_TOKEN environment variables
//   - SSH URLs: tries SSH agent first, then key files (~/.ssh/id_ed25519, id_rsa, id_ecdsa)
//
// Usage:
//
//	repo, err := git.OpenRepository(".")
//	repo.SetLogger(logger)
//	branch, _ := repo.GetCurrentBranch()
//	platform, _ := repo.DetectPlatform()
//	repo.PushBranch(branch)
//
// Thread Safety: [Repository] is not safe for concurrent use.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/internal/security"
	"github.com/sgaunet/bullets"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	// localGitTimeout for local git operations (switch, delete).
	localGitTimeout = 10 * time.Second

	// networkGitTimeout for network git operations (pull, fetch).
	networkGitTimeout = 2 * time.Minute
)

var (
	errMainBranchNotFound  = errors.New("could not determine main branch")
	errHEADNotBranch       = errors.New("HEAD is not pointing to a branch")
	errNoRemoteURLs        = errors.New("no URLs found for origin remote")
	errUnsupportedPlatform = errors.New("repository is not hosted on GitLab or GitHub")
	errStopIteration       = errors.New("stop iteration")
	errNoSSHKeys           = errors.New("no SSH keys found in ~/.ssh")
	errNotGitRepository    = errors.New("not a git repository (or any parent up to mount point)")
)

// GitTimeoutError wraps timeout errors with the name of the operation that timed out
// and the configured timeout duration. Use errors.As to check for this error type.
//
//nolint:revive // Package-qualified name is intentional for clarity
type GitTimeoutError struct {
	Operation string
	Timeout   time.Duration
	Err       error
}

func (e *GitTimeoutError) Error() string {
	return fmt.Sprintf("git %s operation timed out after %v: %v",
		e.Operation, e.Timeout, e.Err)
}

func (e *GitTimeoutError) Unwrap() error {
	return e.Err
}

// noAuthMethod represents no authentication (returns nil).
type noAuthMethod struct{}

func (n *noAuthMethod) Name() string   { return "none" }
func (n *noAuthMethod) String() string { return "none" }

// authMethod wraps transport.AuthMethod to provide a concrete return type.
type authMethod struct {
	method transport.AuthMethod
}

// Repository wraps a go-git repository with authentication and logging.
// It provides both go-git-based and native git operations.
//
// Not safe for concurrent use.
type Repository struct {
	repo    *git.Repository
	gitRoot string // absolute path to git repository root
	auth    transport.AuthMethod
	log     *bullets.Logger
}

// Platform represents a git hosting platform.
type Platform string

const (
	// PlatformGitLab represents GitLab hosting.
	PlatformGitLab Platform = "gitlab"
	// PlatformGitHub represents GitHub hosting.
	PlatformGitHub Platform = "github"
)

// findGitRoot searches for the git repository root starting from the given path.
// It searches upward through parent directories until it finds .git or reaches filesystem root.
// Returns the absolute path to the git repository root or an error if not found.
func findGitRoot(startPath string) (string, error) {
	// Convert to absolute path
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	currentPath := absPath
	for {
		// Check if .git exists (directory or file for worktrees)
		gitPath := filepath.Join(currentPath, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return currentPath, nil
		}

		// Move to parent directory
		parentPath := filepath.Dir(currentPath)

		// Check if we've reached the filesystem root
		if parentPath == currentPath {
			return "", errNotGitRepository
		}

		currentPath = parentPath
	}
}

// OpenRepository opens a git repository at the given path.
// It searches upward from path to find the .git directory and configures
// authentication automatically based on the remote URL.
//
// Parameters:
//   - path: any path within the git repository (absolute or relative)
//
// Returns an error if the path is not within a git repository or authentication setup fails.
func OpenRepository(path string) (*Repository, error) {
	noLog := logger.NoLogger()

	// Find git repository root
	gitRoot, err := findGitRoot(path)
	if err != nil {
		return nil, fmt.Errorf("failed to locate git repository: %w", err)
	}

	// Open repository using found root
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	r := &Repository{
		repo:    repo,
		gitRoot: gitRoot,
		log:     noLog,
	}
	auth, err := getAuth(repo, noLog)
	if err != nil {
		return nil, fmt.Errorf("failed to setup authentication: %w", err)
	}

	// Convert authMethod to transport.AuthMethod (nil for noAuthMethod)
	var finalAuth transport.AuthMethod
	if auth != nil && auth.method != nil {
		if _, isNoAuth := auth.method.(*noAuthMethod); !isNoAuth {
			finalAuth = auth.method
		}
	}

	r.auth = finalAuth
	return r, nil
}

// SetLogger sets the logger for the repository.
func (r *Repository) SetLogger(logger *bullets.Logger) {
	r.log = logger
	r.log.Debug("Opening git repository")
}

// getAuth determines the appropriate authentication method based on the remote URL.
func getAuth(repo *git.Repository, logger *bullets.Logger) (*authMethod, error) {
	remote, err := repo.Remote("origin")
	if err != nil {
		return nil, fmt.Errorf("failed to get origin remote: %w", err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return nil, errNoRemoteURLs
	}

	url := urls[0]
	logger.Debug("Determining authentication method for URL: " + url)

	// Check if it's an HTTPS URL and if tokens are available
	if strings.HasPrefix(url, "https://") {
		return getHTTPSAuth(url, logger)
	}

	// For SSH URLs, setup SSH authentication
	if strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://") {
		return setupSSHAuth(logger)
	}

	logger.Debug("No authentication required")
	return &authMethod{method: &noAuthMethod{}}, nil // No authentication needed
}

// getHTTPSAuth returns HTTP authentication for HTTPS URLs.
func getHTTPSAuth(url string, logger *bullets.Logger) (*authMethod, error) {
	if strings.Contains(url, "gitlab.com") {
		if tokenStr := os.Getenv("GITLAB_TOKEN"); tokenStr != "" {
			token := security.NewSecureToken(tokenStr)
			security.DebugAuth(logger, "GitLab", map[string]string{
				"method": "token",
				"url":    url,
			})
			return &authMethod{method: &http.BasicAuth{
				Username: "oauth2",
				Password: token.Value(), // Extract actual token only for authentication
			}}, nil
		}
		logger.Debug("GITLAB_TOKEN not found")
	} else if strings.Contains(url, "github.com") {
		if tokenStr := os.Getenv("GITHUB_TOKEN"); tokenStr != "" {
			token := security.NewSecureToken(tokenStr)
			security.DebugAuth(logger, "GitHub", map[string]string{
				"method": "token",
				"url":    url,
			})
			return &authMethod{method: &http.BasicAuth{
				Username: "x-access-token",
				Password: token.Value(), // Extract actual token only for authentication
			}}, nil
		}
		logger.Debug("GITHUB_TOKEN not found")
	}
	return &authMethod{method: &noAuthMethod{}}, nil // No token available, try without auth
}

// setupSSHAuth configures SSH authentication using the user's SSH keys.
// It tries SSH agent first (which handles passphrase-protected keys),
// then falls back to reading key files directly.
func setupSSHAuth(logger *bullets.Logger) (*authMethod, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Setup known_hosts callback for both agent and file-based auth
	var hostKeyCallback gossh.HostKeyCallback
	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")
	if _, err := os.Stat(knownHostsPath); err == nil {
		callback, err := knownhosts.New(knownHostsPath)
		if err == nil {
			hostKeyCallback = callback
		}
	}

	// Priority 1: Try SSH agent first (handles passphrase-protected keys)
	logger.Debug("Trying SSH agent authentication")
	sshAgentAuth, err := ssh.NewSSHAgentAuth("git")
	if err == nil {
		if hostKeyCallback != nil {
			sshAgentAuth.HostKeyCallback = hostKeyCallback
		}
		logger.Debug("SSH agent authentication configured successfully")
		return &authMethod{method: sshAgentAuth}, nil
	}
	logger.Debug(fmt.Sprintf("SSH agent not available: %v", err))

	// Priority 2: Fall back to reading key files directly
	keyFiles := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519"),
		filepath.Join(homeDir, ".ssh", "id_rsa"),
		filepath.Join(homeDir, ".ssh", "id_ecdsa"),
	}

	var sshAuth *ssh.PublicKeys
	for _, keyFile := range keyFiles {
		if _, err := os.Stat(keyFile); err == nil {
			security.DebugSSHKey(logger, keyFile, false) // Log attempt with masked path
			// #nosec G304 - Reading SSH keys from standard locations is intentional
			sshAuth, err = ssh.NewPublicKeysFromFile("git", keyFile, "")
			if err != nil {
				sanitizedErr := security.SanitizeString(err.Error())
				logger.Debug("Failed to load SSH key: " + sanitizedErr)
				// Try next key file if this one fails
				continue
			}

			if hostKeyCallback != nil {
				sshAuth.HostKeyCallback = hostKeyCallback
			}

			security.DebugSSHKey(logger, keyFile, true) // Log success with masked path
			return &authMethod{method: sshAuth}, nil
		}
	}

	return nil, errNoSSHKeys
}

// GetMainBranch determines the main branch name by checking the remote HEAD reference.
// Falls back to checking for "main" and "master" branches if the remote HEAD is unavailable.
//
// Returns errMainBranchNotFound if neither method succeeds.
func (r *Repository) GetMainBranch() (string, error) {
	r.log.Debug("Determining main branch")
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to get origin remote: %w", err)
	}

	refs, err := remote.List(&git.ListOptions{
		Auth: r.auth,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list remote references: %w", err)
	}

	for _, ref := range refs {
		if ref.Name() == plumbing.HEAD {
			// Extract branch name from symbolic reference
			target := ref.Target()
			if target.IsBranch() {
				mainBranch := target.Short()
				r.log.Debug("Main branch found: " + mainBranch)
				return mainBranch, nil
			}
		}
	}

	// Fallback to common default branches
	for _, defaultBranch := range []string{"main", "master"} {
		if r.branchExists(defaultBranch) {
			r.log.Debug("Main branch found (fallback): " + defaultBranch)
			return defaultBranch, nil
		}
	}

	return "", errMainBranchNotFound
}

// GetCurrentBranch returns the short name of the currently checked out branch.
//
// Returns errHEADNotBranch if HEAD is in detached state.
func (r *Repository) GetCurrentBranch() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD reference: %w", err)
	}

	if !head.Name().IsBranch() {
		return "", errHEADNotBranch
	}

	return head.Name().Short(), nil
}

// HasStagedChanges checks if there are any staged changes in the repository.
func (r *Repository) HasStagedChanges() (bool, error) {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get repository status: %w", err)
	}

	for _, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified {
			return true, nil
		}
	}

	return false, nil
}

// DetectPlatform determines if the repository is hosted on GitLab or GitHub
// by inspecting the origin remote URL for "gitlab.com" or "github.com".
//
// Returns [PlatformGitLab] or [PlatformGitHub].
// Returns errUnsupportedPlatform if the URL contains neither.
func (r *Repository) DetectPlatform() (Platform, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to get origin remote: %w", err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", errNoRemoteURLs
	}

	url := urls[0]
	if strings.Contains(url, "gitlab.com") {
		return PlatformGitLab, nil
	}
	if strings.Contains(url, "github.com") {
		return PlatformGitHub, nil
	}

	return "", errUnsupportedPlatform
}

// PushBranch pushes the specified branch to the origin remote using go-git.
// If the branch is already up to date, no error is returned.
//
// Parameters:
//   - branchName: the local branch name to push
func (r *Repository) PushBranch(branchName string) error {
	r.log.Debug("Pushing branch: " + branchName)
	err := r.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/" + branchName + ":refs/heads/" + branchName),
		},
		Auth: r.auth,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to push branch: %w", err)
	}
	r.log.Debug("Branch pushed successfully: " + branchName)
	return nil
}

// SwitchBranch switches to the specified branch using native "git switch".
// This will fail if there are local changes that would conflict with the switch,
// forcing the user to handle conflicts manually (matching auto-mr.sh behavior).
// Untracked files are preserved.
//
// Parameters:
//   - ctx: context for cancellation (further bounded by localGitTimeout)
//   - branchName: the branch to switch to
//
// Returns [*GitTimeoutError] if the operation exceeds localGitTimeout (10s).
func (r *Repository) SwitchBranch(ctx context.Context, branchName string) error {
	r.log.Debug("Switching to branch using git switch: " + branchName)

	// Use native git switch command to match shell script behavior
	// This preserves untracked files and fails on conflicts (desired behavior)
	ctx, cancel := context.WithTimeout(ctx, localGitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "switch", branchName)
	cmd.Dir = r.gitRoot // Set working directory to git root
	output, err := cmd.CombinedOutput()

	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &GitTimeoutError{
			Operation: "switch",
			Timeout:   localGitTimeout,
			Err:       err,
		}
	}

	if err != nil {
		//nolint:wrapcheck // Error is sanitized to prevent token leakage
		return security.SanitizeError(fmt.Errorf("failed to switch branch: %w\nOutput: %s", err, string(output)))
	}

	r.log.Debug("Branch switched successfully: " + branchName)
	return nil
}

// Pull fetches and merges changes from the remote tracking branch using native "git pull".
//
// Parameters:
//   - ctx: context for cancellation (further bounded by networkGitTimeout)
//
// Returns [*GitTimeoutError] if the operation exceeds networkGitTimeout (2m).
func (r *Repository) Pull(ctx context.Context) error {
	r.log.Debug("Pulling changes using git pull")

	// Use native git pull command to match shell script behavior
	ctx, cancel := context.WithTimeout(ctx, networkGitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "pull")
	cmd.Dir = r.gitRoot // Set working directory to git root
	output, err := cmd.CombinedOutput()

	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &GitTimeoutError{
			Operation: "pull",
			Timeout:   networkGitTimeout,
			Err:       err,
		}
	}

	if err != nil {
		//nolint:wrapcheck // Error is sanitized to prevent token leakage
		return security.SanitizeError(fmt.Errorf("failed to pull: %w\nOutput: %s", err, string(output)))
	}

	r.log.Debug("Pull completed successfully")
	return nil
}

// DeleteBranch force-deletes the specified local branch using native "git branch -D".
//
// Parameters:
//   - ctx: context for cancellation (further bounded by localGitTimeout)
//   - branchName: the local branch to delete
//
// Returns [*GitTimeoutError] if the operation exceeds localGitTimeout (10s).
func (r *Repository) DeleteBranch(ctx context.Context, branchName string) error {
	r.log.Debug("Deleting branch using git branch -D: " + branchName)

	// Use native git branch -D to force delete (matching shell script behavior)
	ctx, cancel := context.WithTimeout(ctx, localGitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "branch", "-D", branchName)
	cmd.Dir = r.gitRoot // Set working directory to git root
	output, err := cmd.CombinedOutput()

	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &GitTimeoutError{
			Operation: "delete branch",
			Timeout:   localGitTimeout,
			Err:       err,
		}
	}

	if err != nil {
		//nolint:wrapcheck // Error is sanitized to prevent token leakage
		return security.SanitizeError(fmt.Errorf("failed to delete branch: %w\nOutput: %s", err, string(output)))
	}

	r.log.Debug("Branch deleted successfully: " + branchName)
	return nil
}

// FetchAndPrune fetches from origin and prunes deleted remote branches using native "git fetch --prune".
//
// Parameters:
//   - ctx: context for cancellation (further bounded by networkGitTimeout)
//
// Returns [*GitTimeoutError] if the operation exceeds networkGitTimeout (2m).
func (r *Repository) FetchAndPrune(ctx context.Context) error {
	r.log.Debug("Fetching and pruning using git fetch --prune")

	// Use native git fetch --prune to match shell script behavior
	ctx, cancel := context.WithTimeout(ctx, networkGitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "fetch", "--prune")
	cmd.Dir = r.gitRoot // Set working directory to git root
	output, err := cmd.CombinedOutput()

	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &GitTimeoutError{
			Operation: "fetch and prune",
			Timeout:   networkGitTimeout,
			Err:       err,
		}
	}

	if err != nil {
		//nolint:wrapcheck // Error is sanitized to prevent token leakage
		return security.SanitizeError(fmt.Errorf("failed to fetch and prune: %w\nOutput: %s", err, string(output)))
	}

	r.log.Debug("Fetch and prune completed successfully")
	return nil
}

// GetLatestCommitMessage returns the full commit message of the current HEAD commit.
func (r *Repository) GetLatestCommitMessage() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD reference: %w", err)
	}

	commit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		return "", fmt.Errorf("failed to get commit object: %w", err)
	}

	return commit.Message, nil
}

// GetCommitsSinceMain returns all commits on the current branch since it diverged from the main branch.
// Iteration stops when the main branch HEAD commit is reached.
//
// Parameters:
//   - mainBranch: the base branch name (e.g., "main")
func (r *Repository) GetCommitsSinceMain(mainBranch string) ([]*object.Commit, error) {
	currentHead, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get current HEAD: %w", err)
	}

	mainRef, err := r.repo.Reference(plumbing.NewBranchReferenceName(mainBranch), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get main branch reference: %w", err)
	}

	commitIter, err := r.repo.Log(&git.LogOptions{
		From: currentHead.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}
	defer commitIter.Close()

	var commits []*object.Commit
	err = commitIter.ForEach(func(commit *object.Commit) error {
		if commit.Hash == mainRef.Hash() {
			return errStopIteration // Found the main branch commit
		}
		commits = append(commits, commit)
		return nil
	})

	if err != nil && !errors.Is(err, errStopIteration) {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}

// GetRemoteURL returns the first URL configured for the specified remote.
//
// Parameters:
//   - remoteName: the remote name (e.g., "origin")
//
// Returns errNoRemoteURLs if the remote has no configured URLs.
func (r *Repository) GetRemoteURL(remoteName string) (string, error) {
	remote, err := r.repo.Remote(remoteName)
	if err != nil {
		return "", fmt.Errorf("failed to get remote %s: %w", remoteName, err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", fmt.Errorf("%w for remote %s", errNoRemoteURLs, remoteName)
	}

	return urls[0], nil
}

// GoGitRepository returns the underlying go-git Repository.
// This is used by the commits package to retrieve commit history.
func (r *Repository) GoGitRepository() *git.Repository {
	return r.repo
}

func (r *Repository) branchExists(branchName string) bool {
	_, err := r.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	return err == nil
}
