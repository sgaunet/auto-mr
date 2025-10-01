// Package main provides the entry point for the auto-mr CLI tool.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/internal/ui"
	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/git"
	"github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/spf13/cobra"

	gogithub "github.com/google/go-github/v69/github"
	gogitlab "gitlab.com/gitlab-org/api/client-go"
)

const (
	maxLabelsToSelect     = 3
	pipelineStartupDelay  = 2 * time.Second
	defaultPipelineTimeout = 30 * time.Minute
)

var (
	errOnMainBranch        = errors.New("you are on the main branch. Please checkout to a feature branch")
	errUnsupportedPlatform = errors.New("unsupported platform")
	errPipelineFailed      = errors.New("pipeline failed")
	errWorkflowFailed      = errors.New("workflow failed")
)

var (
	logLevel string
	log      *slog.Logger
)

var rootCmd = &cobra.Command{
	Use:   "auto-mr",
	Short: "Automated merge request tool for GitLab and GitHub",
	Long: `auto-mr automates the process of creating and merging pull/merge requests
on GitLab and GitHub repositories. It handles pipeline waiting, auto-approval,
and branch cleanup.`,
	Run: func(_ *cobra.Command, _ []string) {
		if err := runAutoMR(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info",
		"Set log level (debug, info, warn, error)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runAutoMR() error {
	log = logger.NewLogger(logLevel)
	log.Info("auto-mr starting...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	log.Debug("Configuration loaded successfully")

	repo, err := git.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}
	repo.SetLogger(log)

	mainBranch, currentBranch, err := validateBranches(repo)
	if err != nil {
		return err
	}

	platform, err := repo.DetectPlatform()
	if err != nil {
		return fmt.Errorf("failed to detect platform: %w", err)
	}
	log.Info("Platform detected", "platform", platform)

	if err := prepareRepository(repo, currentBranch); err != nil {
		return err
	}

	title, body, err := getCommitInfo(repo)
	if err != nil {
		return err
	}

	return routeToPlatform(platform, cfg, currentBranch, mainBranch, title, body, repo)
}

func validateBranches(repo *git.Repository) (string, string, error) {
	mainBranch, err := repo.GetMainBranch()
	if err != nil {
		return "", "", fmt.Errorf("failed to get main branch: %w", err)
	}
	log.Info("Main branch identified", "branch", mainBranch)

	currentBranch, err := repo.GetCurrentBranch()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current branch: %w", err)
	}
	log.Info("Current branch", "branch", currentBranch)

	if currentBranch == mainBranch {
		return "", "", errOnMainBranch
	}

	return mainBranch, currentBranch, nil
}

func prepareRepository(repo *git.Repository, currentBranch string) error {
	log.Info("Pushing branch", "branch", currentBranch)
	if err := repo.PushBranch(currentBranch); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}
	log.Info("Branch pushed successfully")
	return nil
}

func getCommitInfo(repo *git.Repository) (string, string, error) {
	commitMessage, err := repo.GetLatestCommitMessage()
	if err != nil {
		return "", "", fmt.Errorf("failed to get commit message: %w", err)
	}

	title := strings.Split(commitMessage, "\n")[0]
	body := commitMessage
	return title, body, nil
}

func routeToPlatform(
	platform git.Platform,
	cfg *config.Config,
	currentBranch, mainBranch, title, body string,
	repo *git.Repository,
) error {
	switch platform {
	case git.PlatformGitLab:
		return handleGitLab(cfg, currentBranch, mainBranch, title, body, repo)
	case git.PlatformGitHub:
		return handleGitHub(cfg, currentBranch, mainBranch, title, body, repo)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedPlatform, platform)
	}
}

func handleGitLab(cfg *config.Config, currentBranch, mainBranch, title, body string, repo *git.Repository) error {
	client, err := initializeGitLabClient(repo)
	if err != nil {
		return err
	}

	selectedLabels, err := selectGitLabLabels(client)
	if err != nil {
		return err
	}

	mr, err := createGitLabMR(client, cfg, currentBranch, mainBranch, title, body, selectedLabels)
	if err != nil {
		return err
	}

	if err := waitAndMergeGitLabMR(client, mr); err != nil {
		return err
	}

	return cleanup(repo, mainBranch, currentBranch)
}

func initializeGitLabClient(repo *git.Repository) (*gitlab.Client, error) {
	client, err := gitlab.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}
	client.SetLogger(log)

	remoteURL, err := repo.GetRemoteURL("origin")
	if err != nil {
		return nil, fmt.Errorf("failed to get remote URL: %w", err)
	}

	if err := client.SetProjectFromURL(remoteURL); err != nil {
		return nil, fmt.Errorf("failed to set GitLab project: %w", err)
	}

	return client, nil
}

func selectGitLabLabels(client *gitlab.Client) ([]string, error) {
	labels, err := client.ListLabels()
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	labelSelector := ui.NewLabelSelector()
	labelSelector.SetLogger(log)
	labelInterfaces := make([]ui.Label, len(labels))
	for i, label := range labels {
		labelInterfaces[i] = &ui.GitLabLabel{Name: label.Name}
	}

	selectedLabels, err := labelSelector.SelectLabels(labelInterfaces, maxLabelsToSelect)
	if err != nil {
		return nil, fmt.Errorf("failed to select labels: %w", err)
	}

	return selectedLabels, nil
}

func createGitLabMR(
	client *gitlab.Client,
	cfg *config.Config,
	currentBranch, mainBranch, title, body string,
	labels []string,
) (*gogitlab.MergeRequest, error) {
	log.Info("Creating merge request...")
	mr, err := client.CreateMergeRequest(
		currentBranch, mainBranch, title, body,
		cfg.GitLab.Assignee, cfg.GitLab.Reviewer, labels,
	)
	if err != nil {
		// Check if error is about MR already existing
		errMsg := err.Error()
		if strings.Contains(errMsg, "already exists") ||
			strings.Contains(errMsg, "Another open merge request already exists") {
			log.Warn("Merge request already exists for branch", "branch", currentBranch)
			// Fetch the existing MR
			existingMR, fetchErr := client.GetMergeRequestByBranch(currentBranch, mainBranch)
			if fetchErr != nil {
				return nil, fmt.Errorf("failed to fetch existing merge request: %w", fetchErr)
			}
			log.Info("Using existing merge request", "url", existingMR.WebURL)
			return existingMR, nil
		}
		return nil, fmt.Errorf("failed to create merge request: %w", err)
	}

	log.Info("Merge request created", "url", mr.WebURL)
	return mr, nil
}

func waitAndMergeGitLabMR(client *gitlab.Client, mr *gogitlab.MergeRequest) error {
	log.Info("Waiting for pipeline to complete...")
	time.Sleep(pipelineStartupDelay)

	status, err := client.WaitForPipeline(defaultPipelineTimeout)
	if err != nil {
		return fmt.Errorf("failed to wait for pipeline: %w", err)
	}

	if status != "success" && status != "" {
		return fmt.Errorf("%w with status: %s", errPipelineFailed, status)
	}

	log.Info("Approving merge request...")
	if err := client.ApproveMergeRequest(mr.IID); err != nil {
		log.Warn("Failed to approve merge request", "error", err)
	}

	log.Info("Merging merge request...")
	if err := client.MergeMergeRequest(mr.IID); err != nil {
		return fmt.Errorf("failed to merge MR: %w", err)
	}

	log.Info("Merge request merged successfully")
	return nil
}

func handleGitHub(cfg *config.Config, currentBranch, mainBranch, title, body string, repo *git.Repository) error {
	client, err := initializeGitHubClient(repo)
	if err != nil {
		return err
	}

	selectedLabels, err := selectGitHubLabels(client)
	if err != nil {
		return err
	}

	pr, err := createGitHubPR(client, cfg, currentBranch, mainBranch, title, body, selectedLabels)
	if err != nil {
		return err
	}

	if err := waitAndMergeGitHubPR(client, pr); err != nil {
		return err
	}

	return cleanup(repo, mainBranch, currentBranch)
}

func initializeGitHubClient(repo *git.Repository) (*github.Client, error) {
	client, err := github.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %w", err)
	}
	client.SetLogger(log)

	remoteURL, err := repo.GetRemoteURL("origin")
	if err != nil {
		return nil, fmt.Errorf("failed to get remote URL: %w", err)
	}

	if err := client.SetRepositoryFromURL(remoteURL); err != nil {
		return nil, fmt.Errorf("failed to set GitHub repository: %w", err)
	}

	return client, nil
}

func selectGitHubLabels(client *github.Client) ([]string, error) {
	labels, err := client.ListLabels()
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	labelSelector := ui.NewLabelSelector()
	labelSelector.SetLogger(log)
	labelInterfaces := make([]ui.Label, len(labels))
	for i, label := range labels {
		labelInterfaces[i] = &ui.GitHubLabel{Name: label.Name}
	}

	selectedLabels, err := labelSelector.SelectLabels(labelInterfaces, maxLabelsToSelect)
	if err != nil {
		return nil, fmt.Errorf("failed to select labels: %w", err)
	}

	return selectedLabels, nil
}

func createGitHubPR(
	client *github.Client,
	cfg *config.Config,
	currentBranch, mainBranch, title, body string,
	labels []string,
) (*gogithub.PullRequest, error) {
	log.Info("Creating pull request...")
	pr, err := client.CreatePullRequest(
		currentBranch, mainBranch, title, body,
		[]string{cfg.GitHub.Assignee},
		[]string{cfg.GitHub.Reviewer},
		labels,
	)
	if err != nil {
		// Check if error is about PR already existing
		if strings.Contains(err.Error(), "pull request already exists") {
			log.Warn("Pull request already exists for branch", "branch", currentBranch)
			// Fetch the existing PR
			existingPR, fetchErr := client.GetPullRequestByBranch(currentBranch, mainBranch)
			if fetchErr != nil {
				return nil, fmt.Errorf("failed to fetch existing pull request: %w", fetchErr)
			}
			log.Info("Using existing pull request", "url", *existingPR.HTMLURL)
			return existingPR, nil
		}
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	log.Info("Pull request created", "url", *pr.HTMLURL)
	return pr, nil
}

func waitAndMergeGitHubPR(client *github.Client, pr *gogithub.PullRequest) error {
	log.Info("Waiting for workflows to complete...")
	time.Sleep(pipelineStartupDelay)

	conclusion, err := client.WaitForWorkflows(defaultPipelineTimeout)
	if err != nil {
		return fmt.Errorf("failed to wait for workflows: %w", err)
	}

	if conclusion != "success" && conclusion != "" {
		return fmt.Errorf("%w with conclusion: %s", errWorkflowFailed, conclusion)
	}

	log.Info("Merging pull request...")
	if err := client.MergePullRequest(*pr.Number, "squash"); err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	log.Info("Pull request merged successfully")
	return nil
}

func cleanup(repo *git.Repository, mainBranch, currentBranch string) error {
	// Switch to main branch
	log.Info("Switching to main branch", "branch", mainBranch)
	if err := repo.SwitchBranch(mainBranch); err != nil {
		return fmt.Errorf("failed to switch to main branch: %w", err)
	}

	// Pull latest changes
	log.Info("Pulling latest changes...")
	if err := repo.Pull(); err != nil {
		return fmt.Errorf("failed to pull changes: %w", err)
	}

	// Fetch and prune
	log.Info("Fetching and pruning...")
	if err := repo.FetchAndPrune(); err != nil {
		return fmt.Errorf("failed to fetch and prune: %w", err)
	}

	// Delete feature branch
	log.Info("Deleting feature branch", "branch", currentBranch)
	if err := repo.DeleteBranch(currentBranch); err != nil {
		return fmt.Errorf("failed to delete branch: %w", err)
	}

	log.Info("auto-mr completed successfully!")
	return nil
}

