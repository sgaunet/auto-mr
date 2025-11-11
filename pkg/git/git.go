// Package git provides git repository operations using go-git library.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/bullets"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	errMainBranchNotFound   = errors.New("could not determine main branch")
	errHEADNotBranch        = errors.New("HEAD is not pointing to a branch")
	errNoRemoteURLs         = errors.New("no URLs found for origin remote")
	errUnsupportedPlatform  = errors.New("repository is not hosted on GitLab or GitHub")
	errStopIteration        = errors.New("stop iteration")
	errNoSSHKeys            = errors.New("no SSH keys found in ~/.ssh")
)

// noAuthMethod represents no authentication (returns nil).
type noAuthMethod struct{}

func (n *noAuthMethod) Name() string   { return "none" }
func (n *noAuthMethod) String() string { return "none" }

// authMethod wraps transport.AuthMethod to provide a concrete return type.
type authMethod struct {
	method transport.AuthMethod
}

// Repository wraps a go-git repository with additional functionality.
type Repository struct {
	repo *git.Repository
	auth transport.AuthMethod
	log  *bullets.Logger
}

// Platform represents a git hosting platform.
type Platform string

const (
	// PlatformGitLab represents GitLab hosting.
	PlatformGitLab Platform = "gitlab"
	// PlatformGitHub represents GitHub hosting.
	PlatformGitHub Platform = "github"
)

// OpenRepository opens a git repository at the given path.
func OpenRepository(path string) (*Repository, error) {
	noLog := logger.NoLogger()
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	r := &Repository{repo: repo, log: noLog}
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
		if token := os.Getenv("GITLAB_TOKEN"); token != "" {
			logger.Debug("Using GitLab token authentication")
			return &authMethod{method: &http.BasicAuth{
				Username: "oauth2",
				Password: token,
			}}, nil
		}
		logger.Debug("GITLAB_TOKEN not found")
	} else if strings.Contains(url, "github.com") {
		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			logger.Debug("Using GitHub token authentication")
			return &authMethod{method: &http.BasicAuth{
				Username: "x-access-token",
				Password: token,
			}}, nil
		}
		logger.Debug("GITHUB_TOKEN not found")
	}
	return &authMethod{method: &noAuthMethod{}}, nil // No token available, try without auth
}

// setupSSHAuth configures SSH authentication using the user's SSH keys.
func setupSSHAuth(logger *bullets.Logger) (*authMethod, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try common SSH key files
	keyFiles := []string{
		filepath.Join(homeDir, ".ssh", "id_rsa"),
		filepath.Join(homeDir, ".ssh", "id_ed25519"),
		filepath.Join(homeDir, ".ssh", "id_ecdsa"),
	}

	var sshAuth *ssh.PublicKeys
	for _, keyFile := range keyFiles {
		if _, err := os.Stat(keyFile); err == nil {
			logger.Debug("Trying SSH key: " + keyFile)
			// #nosec G304 - Reading SSH keys from standard locations is intentional
			sshAuth, err = ssh.NewPublicKeysFromFile("git", keyFile, "")
			if err != nil {
				logger.Debug(fmt.Sprintf("Failed to load SSH key %s: %v", keyFile, err))
				// Try next key file if this one fails
				continue
			}

			// Setup known_hosts callback
			knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")
			if _, err := os.Stat(knownHostsPath); err == nil {
				callback, err := knownhosts.New(knownHostsPath)
				if err == nil {
					sshAuth.HostKeyCallback = callback
				}
			}

			logger.Debug("SSH authentication configured with key: " + keyFile)
			return &authMethod{method: sshAuth}, nil
		}
	}

	return nil, errNoSSHKeys
}

// GetMainBranch determines the main branch name (main or master).
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

// GetCurrentBranch returns the name of the currently checked out branch.
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

// DetectPlatform determines if the repository is hosted on GitLab or GitHub.
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

// PushBranch pushes the specified branch to the origin remote.
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

// SwitchBranch switches to the specified branch using native git command.
// This will fail if there are local changes that would conflict with the switch,
// forcing the user to handle conflicts manually (matching auto-mr.sh behavior).
func (r *Repository) SwitchBranch(branchName string) error {
	r.log.Debug("Switching to branch using git switch: " + branchName)

	// Use native git switch command to match shell script behavior
	// This preserves untracked files and fails on conflicts (desired behavior)
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "switch", branchName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to switch branch: %w\nOutput: %s", err, string(output))
	}

	r.log.Debug("Branch switched successfully: " + branchName)
	return nil
}

// Pull fetches and merges changes from the remote tracking branch using native git command.
func (r *Repository) Pull() error {
	r.log.Debug("Pulling changes using git pull")

	// Use native git pull command to match shell script behavior
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "pull")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to pull: %w\nOutput: %s", err, string(output))
	}

	r.log.Debug("Pull completed successfully")
	return nil
}

// DeleteBranch force-deletes the specified local branch using native git command.
func (r *Repository) DeleteBranch(branchName string) error {
	r.log.Debug("Deleting branch using git branch -D: " + branchName)

	// Use native git branch -D to force delete (matching shell script behavior)
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "branch", "-D", branchName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete branch: %w\nOutput: %s", err, string(output))
	}

	r.log.Debug("Branch deleted successfully: " + branchName)
	return nil
}

// FetchAndPrune fetches from origin and prunes deleted remote branches using native git command.
func (r *Repository) FetchAndPrune() error {
	r.log.Debug("Fetching and pruning using git fetch --prune")

	// Use native git fetch --prune to match shell script behavior
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "fetch", "--prune")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch and prune: %w\nOutput: %s", err, string(output))
	}

	r.log.Debug("Fetch and prune completed successfully")
	return nil
}

// GetLatestCommitMessage returns the commit message of the current HEAD.
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

// GetCommitsSinceMain returns all commits on the current branch since it diverged from main.
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

// GetRemoteURL returns the URL of the specified remote.
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
