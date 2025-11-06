// Package main provides the entry point for the auto-mr CLI tool.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/internal/ui"
	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/git"
	ghclient "github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/sgaunet/bullets"
	"github.com/spf13/cobra"
	gogitlab "gitlab.com/gitlab-org/api/client-go"
)

const (
	maxLabelsToSelect      = 3
	pipelineStartupDelay   = 2 * time.Second
	defaultPipelineTimeout = 30 * time.Minute
)

var (
	errOnMainBranch        = errors.New("you are on the main branch. Please checkout to a feature branch")
	errUnsupportedPlatform = errors.New("unsupported platform")
	errPipelineFailed      = errors.New("pipeline failed")
	errWorkflowFailed      = errors.New("workflow failed")
)

var (
	logLevel    string
	showVersion bool
	squash      bool
	log         *bullets.Logger
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "auto-mr",
	Short: "Automated merge request tool for GitLab and GitHub",
	Long: `auto-mr automates the process of creating and merging pull/merge requests
on GitLab and GitHub repositories. It handles pipeline waiting, auto-approval,
and branch cleanup.`,
	Run: func(_ *cobra.Command, _ []string) {
		if showVersion {
			fmt.Println(version)
			os.Exit(0)
		}
		if err := runAutoMR(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info",
		"Set log level (debug, info, warn, error)")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Print version and exit")
	rootCmd.Flags().BoolVar(&squash, "squash", false,
		"Squash commits when merging (default: false, preserves commit history)")
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
	log.Infof("Platform detected: %s", platform)

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
	log.Infof("Main branch identified: %s", mainBranch)

	currentBranch, err := repo.GetCurrentBranch()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current branch: %w", err)
	}
	log.Infof("Current branch: %s", currentBranch)

	if currentBranch == mainBranch {
		return "", "", errOnMainBranch
	}

	return mainBranch, currentBranch, nil
}

func prepareRepository(repo *git.Repository, currentBranch string) error {
	log.Infof("Pushing branch: %s", currentBranch)
	log.IncreasePadding()
	if err := repo.PushBranch(currentBranch); err != nil {
		log.DecreasePadding()
		return fmt.Errorf("failed to push branch: %w", err)
	}
	log.Info("Branch pushed successfully")
	log.DecreasePadding()
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

	mr, err := createGitLabMR(client, cfg, currentBranch, mainBranch, title, body, selectedLabels, squash)
	if err != nil {
		return err
	}

	if err := waitAndMergeGitLabMR(client, mr, squash); err != nil {
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
	squash bool,
) (*gogitlab.MergeRequest, error) {
	log.IncreasePadding()
	log.Info("Creating merge request...")
	mr, err := client.CreateMergeRequest(
		currentBranch, mainBranch, title, body,
		cfg.GitLab.Assignee, cfg.GitLab.Reviewer, labels, squash,
	)
	if err != nil {
		// Check if error is about MR already existing
		errMsg := err.Error()
		if strings.Contains(errMsg, "already exists") ||
			strings.Contains(errMsg, "Another open merge request already exists") {
			log.Warnf("Merge request already exists for branch: %s", currentBranch)
			// Fetch the existing MR
			existingMR, fetchErr := client.GetMergeRequestByBranch(currentBranch, mainBranch)
			if fetchErr != nil {
				return nil, fmt.Errorf("failed to fetch existing merge request: %w", fetchErr)
			}
			log.Infof("Using existing merge request: %s", existingMR.WebURL)
			return existingMR, nil
		}
		log.DecreasePadding()
		return nil, fmt.Errorf("failed to create merge request: %w", err)
	}

	log.Infof("Merge request created: %s", mr.WebURL)
	log.DecreasePadding()
	return mr, nil
}

func waitAndMergeGitLabMR(client *gitlab.Client, mr *gogitlab.MergeRequest, squash bool) error {
	time.Sleep(pipelineStartupDelay)

	status, err := client.WaitForPipeline(defaultPipelineTimeout)
	if err != nil {
		return fmt.Errorf("failed to wait for pipeline: %w", err)
	}

	if status != "success" && status != "" {
		return fmt.Errorf("%w with status: %s", errPipelineFailed, status)
	}

	log.Info("Merging merge request...")
	log.IncreasePadding()

	log.Info("Approving merge request...")
	if err := client.ApproveMergeRequest(mr.IID); err != nil {
		log.Warnf("Failed to approve merge request: %v", err)
	}

	if err := client.MergeMergeRequest(mr.IID, squash); err != nil {
		log.DecreasePadding()
		return fmt.Errorf("failed to merge MR: %w", err)
	}

	log.Info("Merge request merged successfully")
	log.DecreasePadding()
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

	if err := waitAndMergeGitHubPR(client, pr, squash); err != nil {
		return err
	}

	return cleanup(repo, mainBranch, currentBranch)
}

func initializeGitHubClient(repo *git.Repository) (*ghclient.Client, error) {
	client, err := ghclient.NewClient()
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

func selectGitHubLabels(client *ghclient.Client) ([]string, error) {
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
	client *ghclient.Client,
	cfg *config.Config,
	currentBranch, mainBranch, title, body string,
	labels []string,
) (*github.PullRequest, error) {
	log.IncreasePadding()
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
			log.Warnf("Pull request already exists for branch: %s", currentBranch)
			// Fetch the existing PR
			existingPR, fetchErr := client.GetPullRequestByBranch(currentBranch, mainBranch)
			if fetchErr != nil {
				return nil, fmt.Errorf("failed to fetch existing pull request: %w", fetchErr)
			}
			log.Infof("Using existing pull request: %s", *existingPR.HTMLURL)
			return existingPR, nil
		}
		log.DecreasePadding()
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	log.Infof("Pull request created: %s", *pr.HTMLURL)
	log.DecreasePadding()
	return pr, nil
}

func waitAndMergeGitHubPR(client *ghclient.Client, pr *github.PullRequest, squash bool) error {
	time.Sleep(pipelineStartupDelay)

	conclusion, err := client.WaitForWorkflows(defaultPipelineTimeout)
	if err != nil {
		return fmt.Errorf("failed to wait for workflows: %w", err)
	}

	if conclusion != "success" && conclusion != "" {
		return fmt.Errorf("%w with conclusion: %s", errWorkflowFailed, conclusion)
	}

	log.Info("Merging pull request...")
	log.IncreasePadding()

	mergeMethod := ghclient.GetMergeMethod(squash)
	if err := client.MergePullRequest(*pr.Number, mergeMethod); err != nil {
		log.DecreasePadding()
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	log.Info("Pull request merged successfully")

	// Delete remote branch after successful merge (matching shell script behavior)
	log.Infof("Deleting remote branch: %s", *pr.Head.Ref)
	if err := client.DeleteBranch(*pr.Head.Ref); err != nil {
		log.Warnf("Failed to delete remote branch: %v", err)
		// Don't fail the entire operation if branch deletion fails
	}

	log.DecreasePadding()
	return nil
}

func cleanup(repo *git.Repository, mainBranch, currentBranch string) error {
	log.Info("Cleanup...")
	log.IncreasePadding()

	// Switch to main branch
	log.Infof("Switching to main branch: %s", mainBranch)
	if err := repo.SwitchBranch(mainBranch); err != nil {
		log.DecreasePadding()
		return fmt.Errorf(
			"failed to switch to main branch: %w\n\n"+
				"If you have local changes that conflict, please handle them manually:\n"+
				"  - Commit your changes, or\n"+
				"  - Stash your changes with: git stash\n"+
				"  - Then run: git switch %s",
			err, mainBranch)
	}

	// Pull latest changes
	log.Info("Pulling latest changes...")
	if err := repo.Pull(); err != nil {
		log.DecreasePadding()
		return fmt.Errorf("failed to pull changes: %w\n\nPlease resolve any conflicts manually and run: git pull", err)
	}

	// Fetch and prune
	log.Info("Fetching and pruning...")
	if err := repo.FetchAndPrune(); err != nil {
		log.DecreasePadding()
		return fmt.Errorf("failed to fetch and prune: %w", err)
	}

	// Delete feature branch
	log.Infof("Deleting feature branch: %s", currentBranch)
	if err := repo.DeleteBranch(currentBranch); err != nil {
		log.DecreasePadding()
		return fmt.Errorf("failed to delete branch: %w", err)
	}

	log.DecreasePadding()
	log.Info("auto-mr completed successfully!")
	return nil
}
