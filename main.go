package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sgaunet/auto-mr/internal/ui"
	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/git"
	"github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "auto-mr",
	Short: "Automated merge request tool for GitLab and GitHub",
	Long: `auto-mr automates the process of creating and merging pull/merge requests
on GitLab and GitHub repositories. It handles pipeline waiting, auto-approval,
and branch cleanup.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runAutoMR(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runAutoMR() error {
	fmt.Println("auto-mr starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Open git repository
	repo, err := git.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get main and current branch names
	mainBranch, err := repo.GetMainBranch()
	if err != nil {
		return fmt.Errorf("failed to get main branch: %w", err)
	}
	fmt.Printf("Main branch: %s\n", mainBranch)

	currentBranch, err := repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	fmt.Printf("Current branch: %s\n", currentBranch)

	// Check if we're on the main branch
	if currentBranch == mainBranch {
		return fmt.Errorf("you are on the main branch. Please checkout to a feature branch")
	}

	// Check for staged changes
	hasStagedChanges, err := repo.HasStagedChanges()
	if err != nil {
		return fmt.Errorf("failed to check staged changes: %w", err)
	}

	if hasStagedChanges {
		return fmt.Errorf("there are changes in the staged area. Please commit them")
	}

	// Determine platform
	platform, err := repo.DetectPlatform()
	if err != nil {
		return fmt.Errorf("failed to detect platform: %w", err)
	}
	fmt.Printf("Platform: %s\n", platform)

	// Push current branch
	fmt.Printf("Pushing branch %s...\n", currentBranch)
	if err := repo.PushBranch(currentBranch); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}

	// Get latest commit message for MR/PR title and body
	commitMessage, err := repo.GetLatestCommitMessage()
	if err != nil {
		return fmt.Errorf("failed to get commit message: %w", err)
	}

	title := strings.Split(commitMessage, "\n")[0]
	body := commitMessage

	// Process based on platform
	switch platform {
	case git.PlatformGitLab:
		return handleGitLab(cfg, currentBranch, mainBranch, title, body, repo)
	case git.PlatformGitHub:
		return handleGitHub(cfg, currentBranch, mainBranch, title, body, repo)
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
}

func handleGitLab(cfg *config.Config, currentBranch, mainBranch, title, body string, repo *git.Repository) error {
	// Initialize GitLab client
	client, err := gitlab.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create GitLab client: %w", err)
	}

	// Set project from git remote URL
	remoteURL, err := repo.GetRemoteURL("origin")
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %w", err)
	}

	if err := client.SetProjectFromURL(remoteURL); err != nil {
		return fmt.Errorf("failed to set GitLab project: %w", err)
	}

	// Get labels and let user select
	labels, err := client.ListLabels()
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	labelSelector := ui.NewLabelSelector()
	labelInterfaces := make([]ui.Label, len(labels))
	for i, label := range labels {
		labelInterfaces[i] = &ui.GitLabLabel{Name: label.Name}
	}

	selectedLabels, err := labelSelector.SelectLabels(labelInterfaces, 3)
	if err != nil {
		return fmt.Errorf("failed to select labels: %w", err)
	}

	// Create merge request
	fmt.Println("Creating merge request...")
	mr, err := client.CreateMergeRequest(
		currentBranch,
		mainBranch,
		title,
		body,
		cfg.GitLab.Assignee,
		cfg.GitLab.Reviewer,
		selectedLabels,
	)
	if err != nil {
		return fmt.Errorf("failed to create merge request: %w", err)
	}

	fmt.Printf("Merge request created: %s\n", mr.WebURL)

	// Wait for pipeline
	fmt.Println("Waiting for pipeline to complete...")
	time.Sleep(2 * time.Second) // Give time for pipeline to start

	status, err := client.WaitForPipeline(30 * time.Minute)
	if err != nil {
		return fmt.Errorf("failed to wait for pipeline: %w", err)
	}

	if status != "success" && status != "" {
		return fmt.Errorf("pipeline failed with status: %s", status)
	}

	// Approve MR
	fmt.Println("Approving merge request...")
	if err := client.ApproveMergeRequest(mr.IID); err != nil {
		fmt.Printf("Warning: failed to approve merge request: %v\n", err)
	}

	// Merge MR
	fmt.Println("Merging merge request...")
	if err := client.MergeMergeRequest(mr.IID); err != nil {
		return fmt.Errorf("failed to merge MR: %w", err)
	}

	// Cleanup
	return cleanup(repo, mainBranch, currentBranch)
}

func handleGitHub(cfg *config.Config, currentBranch, mainBranch, title, body string, repo *git.Repository) error {
	// Initialize GitHub client
	client, err := github.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}

	// Set repository from git remote URL
	remoteURL, err := repo.GetRemoteURL("origin")
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %w", err)
	}

	if err := client.SetRepositoryFromURL(remoteURL); err != nil {
		return fmt.Errorf("failed to set GitHub repository: %w", err)
	}

	// Get labels and let user select
	labels, err := client.ListLabels()
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	labelSelector := ui.NewLabelSelector()
	labelInterfaces := make([]ui.Label, len(labels))
	for i, label := range labels {
		labelInterfaces[i] = &ui.GitHubLabel{Name: label.Name}
	}

	selectedLabels, err := labelSelector.SelectLabels(labelInterfaces, 3)
	if err != nil {
		return fmt.Errorf("failed to select labels: %w", err)
	}

	// Create pull request
	fmt.Println("Creating pull request...")
	pr, err := client.CreatePullRequest(
		currentBranch,
		mainBranch,
		title,
		body,
		[]string{cfg.GitHub.Assignee},
		[]string{cfg.GitHub.Reviewer},
		selectedLabels,
	)
	if err != nil {
		return fmt.Errorf("failed to create pull request: %w", err)
	}

	fmt.Printf("Pull request created: %s\n", *pr.HTMLURL)

	// Wait for workflows
	fmt.Println("Waiting for workflows to complete...")
	time.Sleep(2 * time.Second) // Give time for workflow to start

	conclusion, err := client.WaitForWorkflows(30 * time.Minute)
	if err != nil {
		return fmt.Errorf("failed to wait for workflows: %w", err)
	}

	if conclusion != "success" && conclusion != "" {
		return fmt.Errorf("workflow failed with conclusion: %s", conclusion)
	}

	// Merge PR
	fmt.Println("Merging pull request...")
	if err := client.MergePullRequest(*pr.Number, "squash"); err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	// Cleanup
	return cleanup(repo, mainBranch, currentBranch)
}

func cleanup(repo *git.Repository, mainBranch, currentBranch string) error {
	// Switch to main branch
	fmt.Printf("Switching to %s branch...\n", mainBranch)
	if err := repo.SwitchBranch(mainBranch); err != nil {
		return fmt.Errorf("failed to switch to main branch: %w", err)
	}

	// Pull latest changes
	fmt.Println("Pulling latest changes...")
	if err := repo.Pull(); err != nil {
		return fmt.Errorf("failed to pull changes: %w", err)
	}

	// Fetch and prune
	fmt.Println("Fetching and pruning...")
	if err := repo.FetchAndPrune(); err != nil {
		return fmt.Errorf("failed to fetch and prune: %w", err)
	}

	// Delete feature branch
	fmt.Printf("Deleting branch %s...\n", currentBranch)
	if err := repo.DeleteBranch(currentBranch); err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}

	fmt.Println("auto-mr completed successfully!")
	return nil
}

