// Package commits provides commit history retrieval and message selection for auto-mr.
package commits

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

const (
	// MaxCommitsToRetrieve limits the number of commits to retrieve from history.
	MaxCommitsToRetrieve = 1000
	// DefaultShortHashLength is the default length for abbreviated commit hashes.
	DefaultShortHashLength = 7
)

var (
	// errStopIteration is used internally to stop commit iteration.
	errStopIteration = errors.New("stop iteration")
)

// Retriever handles commit history retrieval and message selection.
type Retriever struct {
	repo   *git.Repository
	logger *slog.Logger
}

// NewRetriever creates a new commit retriever for the given repository.
func NewRetriever(repo *git.Repository) *Retriever {
	return &Retriever{
		repo:   repo,
		logger: slog.Default(),
	}
}

// SetLogger sets the logger for the retriever.
func (r *Retriever) SetLogger(logger *slog.Logger) {
	r.logger = logger
}

// GetCommits retrieves all commits from the specified branch.
// Returns empty slice if branch has no commits.
// Returns error if branch doesn't exist or git operation fails.
func (r *Retriever) GetCommits(branch string) ([]Commit, error) {
	r.logger.Debug("retrieving commits from branch", "branch", branch)

	// Get reference for the branch
	ref, err := r.repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get reference for branch %s: %w", branch, err)
	}

	// Get commit iterator starting from branch HEAD
	commitIter, err := r.repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log for branch %s: %w", branch, err)
	}

	commits := make([]Commit, 0)
	err = commitIter.ForEach(func(c *object.Commit) error {
		// Stop if we've reached the limit
		if len(commits) >= MaxCommitsToRetrieve {
			return storer.ErrStop
		}

		// Parse and add commit
		commit := ParseCommit(c)
		commits = append(commits, commit)

		return nil
	})

	if err != nil && !errors.Is(err, storer.ErrStop) {
		return nil, fmt.Errorf("failed to iterate commits for branch %s: %w", branch, err)
	}

	r.logger.Debug("retrieved commits", "branch", branch, "count", len(commits))

	if len(commits) == 0 {
		return nil, ErrNoCommits
	}

	return commits, nil
}

// GetCommitsSinceBranch retrieves commits from currentBranch since it diverged from baseBranch.
// Only returns commits unique to currentBranch (not present in baseBranch).
// Returns empty slice if no commits exist since divergence.
// Returns error if branches don't exist or git operation fails.
func (r *Retriever) GetCommitsSinceBranch(currentBranch, baseBranch string) ([]Commit, error) {
	r.logger.Debug("retrieving commits since branch divergence",
		"currentBranch", currentBranch,
		"baseBranch", baseBranch)

	// Get reference for current branch
	currentRef, err := r.repo.Reference(plumbing.NewBranchReferenceName(currentBranch), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get reference for branch %s: %w", currentBranch, err)
	}

	// Get reference for base branch
	baseRef, err := r.repo.Reference(plumbing.NewBranchReferenceName(baseBranch), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get reference for base branch %s: %w", baseBranch, err)
	}

	// Get commit iterator starting from current branch HEAD
	commitIter, err := r.repo.Log(&git.LogOptions{From: currentRef.Hash()})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log for branch %s: %w", currentBranch, err)
	}

	commits := make([]Commit, 0)
	err = commitIter.ForEach(func(c *object.Commit) error {
		// Stop if we've reached the base branch commit (divergence point)
		if c.Hash == baseRef.Hash() {
			return errStopIteration
		}

		// Stop if we've reached the limit
		if len(commits) >= MaxCommitsToRetrieve {
			return storer.ErrStop
		}

		// Parse and add commit
		commit := ParseCommit(c)
		commits = append(commits, commit)

		return nil
	})

	if err != nil && !errors.Is(err, errStopIteration) && !errors.Is(err, storer.ErrStop) {
		return nil, fmt.Errorf("failed to iterate commits for branch %s: %w", currentBranch, err)
	}

	r.logger.Debug("retrieved commits since divergence",
		"currentBranch", currentBranch,
		"baseBranch", baseBranch,
		"count", len(commits))

	if len(commits) == 0 {
		return nil, ErrNoCommits
	}

	return commits, nil
}

// GetMessageForMR determines which commit message to use for MR/PR.
// Handles auto-selection, interactive selection, and manual override.
// Only retrieves commits unique to the feature branch (since divergence from mainBranch).
// Returns ErrNoCommits if no valid commits exist.
// Returns ErrAllCommitsInvalid if all commits are merge commits or have empty messages.
func (r *Retriever) GetMessageForMR(branch, mainBranch, msgFlagValue string) (MessageSelection, error) {
	r.logger.Debug("getting message for MR/PR",
		"branch", branch,
		"mainBranch", mainBranch,
		"manual_msg", msgFlagValue != "")

	// Priority 1: Manual override flag
	if msgFlagValue != "" {
		title, body := ParseCommitMessage(msgFlagValue)
		return MessageSelection{
			Title:            title,
			Body:             body,
			SourceCommitHash: "",
			SelectionMethod:  SelectionManual,
			ManualOverride:   true,
		}, nil
	}

	// Priority 2: Retrieve commits from branch (only commits since divergence from main)
	allCommits, err := r.GetCommitsSinceBranch(branch, mainBranch)
	if err != nil {
		return MessageSelection{}, err
	}

	// Filter valid commits
	validCommits := FilterValidCommits(allCommits)

	if len(validCommits) == 0 {
		return MessageSelection{}, ErrAllCommitsInvalid
	}

	// Priority 3: Auto-select single commit
	if len(validCommits) == 1 {
		commit := validCommits[0]
		r.logger.Debug("auto-selecting single commit", "hash", commit.ShortHash, "title", commit.Title)

		return MessageSelection{
			Title:            commit.Title,
			Body:             commit.Body,
			SourceCommitHash: commit.Hash,
			SelectionMethod:  SelectionAuto,
			ManualOverride:   false,
		}, nil
	}

	// Priority 4: Interactive selection would go here (Phase 4)
	// For now, return error indicating multiple commits require interactive selection
	return MessageSelection{}, fmt.Errorf("%w: found %d commits", ErrMultipleCommitsFound, len(validCommits))
}
