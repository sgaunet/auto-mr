package git

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Repository struct {
	repo *git.Repository
}

type Platform string

const (
	PlatformGitLab Platform = "gitlab"
	PlatformGitHub Platform = "github"
)

func OpenRepository(path string) (*Repository, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	return &Repository{repo: repo}, nil
}

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

	return "", fmt.Errorf("could not determine main branch")
}

func (r *Repository) branchExists(branchName string) bool {
	_, err := r.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	return err == nil
}

func (r *Repository) GetCurrentBranch() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD reference: %w", err)
	}

	if !head.Name().IsBranch() {
		return "", fmt.Errorf("HEAD is not pointing to a branch")
	}

	return head.Name().Short(), nil
}

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

func (r *Repository) DetectPlatform() (Platform, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to get origin remote: %w", err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", fmt.Errorf("no URLs found for origin remote")
	}

	url := urls[0]
	if strings.Contains(url, "gitlab.com") {
		return PlatformGitLab, nil
	}
	if strings.Contains(url, "github.com") {
		return PlatformGitHub, nil
	}

	return "", fmt.Errorf("repository is not hosted on GitLab or GitHub")
}

func (r *Repository) PushBranch(branchName string) error {
	return r.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/" + branchName + ":refs/heads/" + branchName),
		},
	})
}

func (r *Repository) SwitchBranch(branchName string) error {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	return worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
}

func (r *Repository) Pull() error {
	worktree, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	return worktree.Pull(&git.PullOptions{
		RemoteName: "origin",
	})
}

func (r *Repository) DeleteBranch(branchName string) error {
	return r.repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(branchName))
}

func (r *Repository) FetchAndPrune() error {
	return r.repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Prune:      true,
	})
}

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
			return fmt.Errorf("stop iteration") // Found the main branch commit
		}
		commits = append(commits, commit)
		return nil
	})

	if err != nil && err.Error() != "stop iteration" {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}

func (r *Repository) GetRemoteURL(remoteName string) (string, error) {
	remote, err := r.repo.Remote(remoteName)
	if err != nil {
		return "", fmt.Errorf("failed to get remote %s: %w", remoteName, err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", fmt.Errorf("no URLs found for remote %s", remoteName)
	}

	return urls[0], nil
}