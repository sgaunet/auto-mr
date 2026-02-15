// Package main provides the entry point for the auto-mr CLI tool.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/internal/ui"
	"github.com/sgaunet/auto-mr/pkg/commits"
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
	errTooManyLabels       = errors.New("too many labels specified")
	errUnsupportedLabelType = errors.New("unsupported label type")
	errLabelNotFound       = errors.New("label not found in repository")
)

var (
	logLevel    string
	showVersion bool
	noSquash    bool
	msg         string
	listLabels  bool   // List available labels and exit
	labels      string // Comma-separated label names
	log         *bullets.Logger
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "auto-mr",
	Short: "Automated merge request tool for GitLab and GitHub",
	Long: `auto-mr automates the process of creating and merging pull/merge requests
on GitLab and GitHub repositories. It handles pipeline waiting, auto-approval,
and branch cleanup.`,
	Run: func(cmd *cobra.Command, _ []string) {
		if showVersion {
			fmt.Println(version)
			os.Exit(0)
		}
		// Determine label selection mode
		useManualLabels := cmd.Flags().Changed("labels")
		manualLabelsValue := labels

		if err := runAutoMR(useManualLabels, manualLabelsValue); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info",
		"Set log level (debug, info, warn, error)")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Print version and exit")
	rootCmd.Flags().BoolVar(&noSquash, "no-squash", false,
		"Disable squash merge and preserve commit history (default: false, squashes commits)")
	rootCmd.Flags().StringVar(&msg, "msg", "",
		"Custom message for MR/PR (overrides commit message selection)")
	rootCmd.Flags().BoolVar(&listLabels, "list-labels", false,
		"List all available labels and exit")
	rootCmd.Flags().StringVar(&labels, "labels", "",
		"Comma-separated label names (e.g., \"bug,enhancement\"). Use empty string to skip labels.")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// formatConfigError provides user-friendly error messages for configuration errors.
func formatConfigError(err error) error {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".config", "auto-mr", "config.yml")

	switch {
	case errors.Is(err, config.ErrConfigNotFound):
		return fmt.Errorf("%w\n\n"+
			"Expected location: %s\n"+
			"Please create a config file with the following structure:\n\n"+
			"gitlab:\n"+
			"  assignee: your-gitlab-username\n"+
			"  reviewer: reviewer-gitlab-username\n"+
			"github:\n"+
			"  assignee: your-github-username\n"+
			"  reviewer: reviewer-github-username",
			err, configPath)

	case errors.Is(err, config.ErrGitLabAssigneeEmpty):
		return fmt.Errorf("%w\n\nConfig file: %s\nAdd: gitlab.assignee", err, configPath)

	case errors.Is(err, config.ErrGitLabReviewerEmpty):
		return fmt.Errorf("%w\n\nConfig file: %s\nAdd: gitlab.reviewer", err, configPath)

	case errors.Is(err, config.ErrGitHubAssigneeEmpty):
		return fmt.Errorf("%w\n\nConfig file: %s\nAdd: github.assignee", err, configPath)

	case errors.Is(err, config.ErrGitHubReviewerEmpty):
		return fmt.Errorf("%w\n\nConfig file: %s\nAdd: github.reviewer", err, configPath)

	case errors.Is(err, config.ErrGitLabAssigneeInvalid),
		errors.Is(err, config.ErrGitLabReviewerInvalid),
		errors.Is(err, config.ErrGitHubAssigneeInvalid),
		errors.Is(err, config.ErrGitHubReviewerInvalid):
		return fmt.Errorf("%w\n\n"+
			"Config file: %s\n"+
			"Usernames must:\n"+
			"  - Contain only letters, numbers, hyphens (-), or underscores (_)\n"+
			"  - Start and end with a letter or number\n"+
			"  - Be between 1 and 39 characters long",
			err, configPath)

	default:
		return fmt.Errorf("failed to load configuration: %w\n\nConfig file: %s", err, configPath)
	}
}

func runAutoMR(useManualLabels bool, manualLabelsValue string) error {
	log = logger.NewLogger(logLevel)
	log.Info("auto-mr starting...")

	cfg, err := config.Load()
	if err != nil {
		return formatConfigError(err)
	}
	log.Debug("Configuration loaded successfully")

	repo, err := git.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}
	repo.SetLogger(log)

	platform, err := repo.DetectPlatform()
	if err != nil {
		return fmt.Errorf("failed to detect platform: %w", err)
	}
	log.Infof("Platform detected: %s", platform)

	// Handle --list-labels flag (list and exit)
	if listLabels {
		return handleListLabels(platform, repo)
	}

	mainBranch, currentBranch, err := validateBranches(repo)
	if err != nil {
		return err
	}

	if err := prepareRepository(repo, currentBranch); err != nil {
		return err
	}

	title, body, err := getCommitInfo(repo)
	if err != nil {
		return err
	}

	return routeToPlatform(platform, cfg, currentBranch, mainBranch, title, body, repo, useManualLabels, manualLabelsValue)
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
	slogLogger := createSlogLogger()

	// Create commit retriever
	retriever := commits.NewRetriever(repo.GoGitRepository())
	retriever.SetLogger(slogLogger)

	// Get current branch name
	currentBranch, err := repo.GetCurrentBranch()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current branch: %w", err)
	}

	// Get main branch name
	mainBranch, err := repo.GetMainBranch()
	if err != nil {
		return "", "", fmt.Errorf("failed to get main branch: %w", err)
	}

	// Get message selection (handles manual override, auto-select, and interactive selection)
	selection, err := retriever.GetMessageForMR(currentBranch, mainBranch, msg)
	if err != nil {
		selection, err = handleInteractiveSelection(retriever, currentBranch, mainBranch, slogLogger, err)
		if err != nil {
			return "", "", err
		}
	}

	return selection.Title, selection.Body, nil
}

func createSlogLogger() *slog.Logger {
	var slogLevel slog.Level
	switch logLevel {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel}))
}

func handleInteractiveSelection(
	retriever *commits.Retriever,
	currentBranch string,
	mainBranch string,
	slogLogger *slog.Logger,
	origErr error,
) (commits.MessageSelection, error) {
	// If multiple commits found, use interactive selector
	if errors.Is(origErr, commits.ErrMultipleCommitsFound) {
		selector := commits.NewSelector(commits.NewRenderer())
		selector.SetLogger(slogLogger)

		// Get commits since divergence from main branch
		allCommits, getErr := retriever.GetCommitsSinceBranch(currentBranch, mainBranch)
		if getErr != nil {
			return commits.MessageSelection{}, fmt.Errorf("failed to get commits: %w", getErr)
		}

		// Use selector for interactive selection
		selection, err := selector.GetMessageForMR(allCommits, msg)
		if err != nil {
			return commits.MessageSelection{}, fmt.Errorf("failed to select commit message: %w", err)
		}
		return selection, nil
	}
	return commits.MessageSelection{}, fmt.Errorf("failed to get commit message: %w", origErr)
}

func routeToPlatform(
	platform git.Platform,
	cfg *config.Config,
	currentBranch, mainBranch, title, body string,
	repo *git.Repository,
	useManualLabels bool,
	manualLabelsValue string,
) error {
	switch platform {
	case git.PlatformGitLab:
		return handleGitLab(cfg, currentBranch, mainBranch, title, body, repo, useManualLabels, manualLabelsValue)
	case git.PlatformGitHub:
		return handleGitHub(cfg, currentBranch, mainBranch, title, body, repo, useManualLabels, manualLabelsValue)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedPlatform, platform)
	}
}

func handleListLabels(platform git.Platform, repo *git.Repository) error {
	remoteURL, err := repo.GetRemoteURL("origin")
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %w", err)
	}

	switch platform {
	case git.PlatformGitLab:
		return listGitLabLabels(remoteURL)
	case git.PlatformGitHub:
		return listGitHubLabels(remoteURL)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedPlatform, platform)
	}
}

func listGitLabLabels(remoteURL string) error {
	client, err := gitlab.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create GitLab client: %w", err)
	}
	client.SetLogger(log)

	if err := client.SetProjectFromURL(remoteURL); err != nil {
		return fmt.Errorf("failed to set GitLab project: %w", err)
	}

	labels, err := client.ListLabels()
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	fmt.Printf("Available labels for GitLab:%s:\n", remoteURL)
	for _, label := range labels {
		fmt.Printf("- %s\n", label.Name)
	}
	fmt.Printf("\nTotal: %d labels\n", len(labels))
	return nil
}

func listGitHubLabels(remoteURL string) error {
	client, err := ghclient.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}
	client.SetLogger(log)

	if err := client.SetRepositoryFromURL(remoteURL); err != nil {
		return fmt.Errorf("failed to set GitHub repository: %w", err)
	}

	labels, err := client.ListLabels()
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	fmt.Printf("Available labels for GitHub:%s:\n", remoteURL)
	for _, label := range labels {
		fmt.Printf("- %s\n", label.Name)
	}
	fmt.Printf("\nTotal: %d labels\n", len(labels))
	return nil
}

func validateManualLabels(availableLabels any, requestedLabels string) ([]string, error) {
	// Handle empty string case (skip labels)
	if requestedLabels == "" {
		return []string{}, nil
	}

	// Parse and clean labels
	cleanedLabels := parseLabels(requestedLabels)

	// Validate max selection limit
	if len(cleanedLabels) > maxLabelsToSelect {
		return nil, fmt.Errorf("%w: %d (max: %d)", errTooManyLabels, len(cleanedLabels), maxLabelsToSelect)
	}

	// Build map of available labels for O(1) lookup
	availableMap, err := buildLabelMap(availableLabels)
	if err != nil {
		return nil, err
	}

	// Check each requested label exists
	for _, label := range cleanedLabels {
		if !availableMap[label] {
			return nil, fmt.Errorf("%w: '%s'. Use --list-labels to see available labels", errLabelNotFound, label)
		}
	}

	return cleanedLabels, nil
}

func parseLabels(requestedLabels string) []string {
	parts := strings.Split(requestedLabels, ",")
	var cleanedLabels []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleanedLabels = append(cleanedLabels, trimmed)
		}
	}
	return cleanedLabels
}

func buildLabelMap(availableLabels any) (map[string]bool, error) {
	availableMap := make(map[string]bool)
	switch labels := availableLabels.(type) {
	case []gitlab.Label:
		for _, label := range labels {
			availableMap[label.Name] = true
		}
	case []ghclient.Label:
		for _, label := range labels {
			availableMap[label.Name] = true
		}
	default:
		return nil, errUnsupportedLabelType
	}
	return availableMap, nil
}

func handleGitLab(
	cfg *config.Config,
	currentBranch, mainBranch, title, body string,
	repo *git.Repository,
	useManualLabels bool,
	manualLabelsValue string,
) error {
	client, err := initializeGitLabClient(repo)
	if err != nil {
		return err
	}

	selectedLabels, err := selectGitLabLabels(client, useManualLabels, manualLabelsValue)
	if err != nil {
		return err
	}

	mr, err := createGitLabMR(client, cfg, currentBranch, mainBranch, title, body, selectedLabels, !noSquash)
	if err != nil {
		return err
	}

	if err := waitAndMergeGitLabMR(client, mr, !noSquash, title); err != nil {
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

func selectGitLabLabels(client *gitlab.Client, useManualSelection bool, manualLabels string) ([]string, error) {
	// Fetch available labels from API
	availableLabels, err := client.ListLabels()
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	// Check if manual selection via flag (Tier 1)
	if useManualSelection {
		log.Debug("Using manual label selection via --labels flag")
		return validateManualLabels(availableLabels, manualLabels)
	}

	// Interactive selection (Tier 2 - existing behavior)
	log.Debug("Using interactive label selection")
	labelSelector := ui.NewLabelSelector()
	labelSelector.SetLogger(log)
	labelInterfaces := make([]ui.Label, len(availableLabels))
	for i, label := range availableLabels {
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
		// Check if error is about MR already existing using typed error
		if errors.Is(err, gitlab.ErrMRAlreadyExists) {
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

func waitAndMergeGitLabMR(client *gitlab.Client, mr *gogitlab.MergeRequest, squash bool, commitTitle string) error {
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

	if err := client.MergeMergeRequest(mr.IID, squash, commitTitle); err != nil {
		log.DecreasePadding()
		return fmt.Errorf("failed to merge MR: %w", err)
	}

	log.Info("Merge request merged successfully")
	log.DecreasePadding()
	return nil
}

func handleGitHub(
	cfg *config.Config,
	currentBranch, mainBranch, title, body string,
	repo *git.Repository,
	useManualLabels bool,
	manualLabelsValue string,
) error {
	client, err := initializeGitHubClient(repo)
	if err != nil {
		return err
	}

	selectedLabels, err := selectGitHubLabels(client, useManualLabels, manualLabelsValue)
	if err != nil {
		return err
	}

	pr, err := createGitHubPR(client, cfg, currentBranch, mainBranch, title, body, selectedLabels)
	if err != nil {
		return err
	}

	if err := waitAndMergeGitHubPR(client, pr, title, !noSquash); err != nil {
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

func selectGitHubLabels(client *ghclient.Client, useManualSelection bool, manualLabels string) ([]string, error) {
	// Fetch available labels from API
	availableLabels, err := client.ListLabels()
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	// Check if manual selection via flag (Tier 1)
	if useManualSelection {
		log.Debug("Using manual label selection via --labels flag")
		return validateManualLabels(availableLabels, manualLabels)
	}

	// Interactive selection (Tier 2 - existing behavior)
	log.Debug("Using interactive label selection")
	labelSelector := ui.NewLabelSelector()
	labelSelector.SetLogger(log)
	labelInterfaces := make([]ui.Label, len(availableLabels))
	for i, label := range availableLabels {
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
		// Check if error is about PR already existing using typed error
		if errors.Is(err, ghclient.ErrPRAlreadyExists) {
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

func waitAndMergeGitHubPR(
	client *ghclient.Client,
	pr *github.PullRequest,
	commitTitle string,
	squash bool,
) error {
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
	if err := client.MergePullRequest(*pr.Number, mergeMethod, commitTitle); err != nil {
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
	defer log.DecreasePadding()

	log.Infof("Switching to main branch: %s", mainBranch)
	report := repo.Cleanup(mainBranch, currentBranch)

	// Display results with status icons
	displayCleanupStatus(report)

	// Check if critical operations succeeded
	if !report.Success() {
		return fmt.Errorf("cleanup failed: %w", report.FirstError())
	}

	// Warn about non-critical failures
	if report.PruneError != nil || report.DeleteError != nil {
		log.Warn("Cleanup completed with warnings (see above)")
	} else {
		log.Info("auto-mr completed successfully!")
	}

	return nil
}

func displayCleanupStatus(report *git.CleanupReport) {
	steps := []struct {
		name      string
		completed bool
		err       error
	}{
		{"Switch to main branch", report.SwitchedBranch, report.SwitchError},
		{"Pull latest changes", report.PulledChanges, report.PullError},
		{"Fetch and prune", report.Pruned, report.PruneError},
		{"Delete feature branch", report.DeletedBranch, report.DeleteError},
	}

	for _, step := range steps {
		icon := getStatusIcon(step.completed, step.err)
		msg := fmt.Sprintf("%s %s", icon, step.name)

		switch {
		case step.err != nil:
			log.Warnf("%s - %v", msg, step.err)
		case step.completed:
			log.Info(msg)
		default:
			log.Info(msg + " - not attempted")
		}
	}
}

func getStatusIcon(completed bool, err error) string {
	if err != nil {
		return "✗" // Failed
	}
	if completed {
		return "✓" // Success
	}
	return "—" // Not attempted
}
