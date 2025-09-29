// Package git provides git repository operations using go-git library.
package git

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	errMainBranchNotFound   = errors.New("could not determine main branch")
	errHEADNotBranch        = errors.New("HEAD is not pointing to a branch")
	errNoRemoteURLs         = errors.New("no URLs found for origin remote")
	errUnsupportedPlatform  = errors.New("repository is not hosted on GitLab or GitHub")
	errStopIteration        = errors.New("stop iteration")
)

// Repository wraps a go-git repository with additional functionality.
type Repository struct {
	repo *git.Repository
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
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	return &Repository{repo: repo}, nil
}

// GetMainBranch determines the main branch name (main or master).
func (r *Repository) GetMainBranch() (string, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to get origin remote: %w", err)
	}

	refs, err := remote.List(&git.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list remote references: %w", err)
	}

	for _, ref := range refs {
		if ref.Name() == plumbing.HEAD {
			// Extract branch name from symbolic reference
			target := ref.Target()
			if target.IsBranch() {
				return target.Short(), nil
			}
		}
	}

	// Fallback to common default branches
	for _, defaultBranch := range []string{"main", "master"} {
		if r.branchExists(defaultBranch) {
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
	err := r.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/" + branchName + ":refs/heads/" + branchName),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}
	return nil
}

// SwitchBranch checks out the specified branch.
func (r *Repository) SwitchBranch(branchName string) error {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
	if err != nil {
		return fmt.Errorf("failed to checkout branch: %w", err)
	}
	return nil
}

// Pull fetches and merges changes from the remote tracking branch.
func (r *Repository) Pull() error {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Pull(&git.PullOptions{
		RemoteName: "origin",
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to pull: %w", err)
	}
	return nil
}

// DeleteBranch deletes the specified local branch.
func (r *Repository) DeleteBranch(branchName string) error {
	err := r.repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(branchName))
	if err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}
	return nil
}

// FetchAndPrune fetches from origin and prunes deleted remote branches.
func (r *Repository) FetchAndPrune() error {
	err := r.repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Prune:      true,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to fetch and prune: %w", err)
	}
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

func (r *Repository) branchExists(branchName string) bool {
	_, err := r.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	return err == nil
}