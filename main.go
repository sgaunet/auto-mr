// Package main provides the entry point for the auto-mr CLI tool.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	autolabels "github.com/sgaunet/auto-mr/internal/labels"
	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/pkg/commits"
	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/git"
	"github.com/sgaunet/auto-mr/pkg/platform"
	"github.com/sgaunet/bullets"
	"github.com/spf13/cobra"
)

const (
	maxLabelsToSelect      = 3
	pipelineStartupDelay   = 2 * time.Second
	defaultPipelineTimeout = 30 * time.Minute
)

var (
	errOnMainBranch  = errors.New("you are on the main branch. Please checkout to a feature branch")
	errPipelineFailed = errors.New("pipeline failed")
	errTooManyLabels  = errors.New("too many labels specified")
	errLabelNotFound  = errors.New("label not found in repository")
)

var (
	logLevel        string
	showVersion     bool
	noSquash        bool
	msg             string
	listLabels      bool   // List available labels and exit
	labels          string // Comma-separated label names
	pipelineTimeout string // Pipeline/workflow timeout duration
	log             *bullets.Logger
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

		if err := runAutoMR(cmd, useManualLabels, manualLabelsValue); err != nil {
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
	rootCmd.Flags().StringVar(&pipelineTimeout, "pipeline-timeout", "",
		"Pipeline/workflow timeout (e.g., \"30m\", \"1h\", \"90m\"). Overrides config file. (default: 30m)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// getPipelineTimeout resolves pipeline timeout from three sources with priority:
// 1. CLI flag --pipeline-timeout (highest priority).
// 2. Config file platform-specific timeout.
// 3. Default timeout (30 minutes).
func getPipelineTimeout(cmd *cobra.Command, platformConfig string) (time.Duration, error) {
	// Priority 1: CLI flag
	if cmd.Flags().Changed("pipeline-timeout") && pipelineTimeout != "" {
		timeout, err := time.ParseDuration(pipelineTimeout)
		if err != nil {
			return 0, fmt.Errorf("invalid --pipeline-timeout: %w", err)
		}
		if timeout < config.MinPipelineTimeout || timeout > config.MaxPipelineTimeout {
			return 0, fmt.Errorf("%w: --pipeline-timeout must be between %v and %v",
				config.ErrInvalidTimeout, config.MinPipelineTimeout, config.MaxPipelineTimeout)
		}
		return timeout, nil
	}

	// Priority 2: Config file
	if platformConfig != "" {
		timeout, parseErr := time.ParseDuration(platformConfig)
		if parseErr != nil {
			// Should not happen after Validate(), but return default as fallback
			log.Warnf("Invalid platform timeout config '%s', using default %v", platformConfig, defaultPipelineTimeout)
			return defaultPipelineTimeout, nil //nolint:nilerr // intentional fallback to default on parse error
		}
		return timeout, nil
	}

	// Priority 3: Default
	return defaultPipelineTimeout, nil
}

// formatConfigError provides user-friendly error messages for configuration errors.
func formatConfigError(err error) error {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".config", "auto-mr", "config.yml")

	// Check for timeout-related errors first
	if timeoutErr := formatTimeoutError(err, configPath); timeoutErr != nil {
		return timeoutErr
	}

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

// formatTimeoutError handles timeout-specific error formatting.
func formatTimeoutError(err error, configPath string) error {
	switch {
	case errors.Is(err, config.ErrInvalidTimeout):
		return fmt.Errorf("%w\n\n"+
			"Config file: %s\n"+
			"pipeline_timeout must be a valid Go duration format:\n"+
			"  Valid: \"30m\", \"1h\", \"1h30m\", \"90m\"\n"+
			"  Invalid: \"30\" (no unit), \"abc\", \"-5m\"",
			err, configPath)

	case errors.Is(err, config.ErrTimeoutTooSmall):
		return fmt.Errorf("%w\n\n"+
			"Config file: %s\n"+
			"pipeline_timeout must be at least 1 minute (1m)",
			err, configPath)

	case errors.Is(err, config.ErrTimeoutTooLarge):
		return fmt.Errorf("%w\n\n"+
			"Config file: %s\n"+
			"pipeline_timeout must be at most 8 hours (8h)",
			err, configPath)

	default:
		return nil // Not a timeout error
	}
}

func runAutoMR(cmd *cobra.Command, useManualLabels bool, manualLabelsValue string) error {
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

	detectedPlatform, err := repo.DetectPlatform()
	if err != nil {
		return fmt.Errorf("failed to detect platform: %w", err)
	}
	log.Infof("Platform detected: %s", detectedPlatform)

	// Handle --list-labels flag (list and exit)
	if listLabels {
		return handleListLabels(detectedPlatform, cfg, repo)
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

	return routeToPlatform(
		cmd, detectedPlatform, cfg, currentBranch, mainBranch, title, body, repo,
		useManualLabels, manualLabelsValue,
	)
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
	cmd *cobra.Command,
	detectedPlatform git.Platform,
	cfg *config.Config,
	currentBranch, mainBranch, title, body string,
	repo *git.Repository,
	useManualLabels bool,
	manualLabelsValue string,
) error {
	provider, err := platform.NewProvider(detectedPlatform, cfg, log)
	if err != nil {
		return fmt.Errorf("failed to create platform client: %w", err)
	}

	remoteURL, err := repo.GetRemoteURL("origin")
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %w", err)
	}

	if err := provider.Initialize(remoteURL); err != nil {
		return fmt.Errorf("failed to initialize %s client: %w", provider.PlatformName(), err)
	}

	return handlePlatform(cmd, provider, currentBranch, mainBranch, title, body, repo,
		useManualLabels, manualLabelsValue)
}

func handlePlatform(
	cmd *cobra.Command,
	provider platform.Provider,
	currentBranch, mainBranch, title, body string,
	repo *git.Repository,
	useManualLabels bool,
	manualLabelsValue string,
) error {
	selectedLabels, err := selectLabels(provider, useManualLabels, manualLabelsValue, title)
	if err != nil {
		return err
	}

	mr, err := createMR(provider, currentBranch, mainBranch, title, body, selectedLabels, !noSquash)
	if err != nil {
		return err
	}

	if err := waitAndMerge(cmd, provider, mr, !noSquash, title); err != nil {
		return err
	}

	ctx := context.Background()
	return cleanup(ctx, repo, mainBranch, currentBranch)
}

func handleListLabels(detectedPlatform git.Platform, cfg *config.Config, repo *git.Repository) error {
	provider, err := platform.NewProvider(detectedPlatform, cfg, log)
	if err != nil {
		return fmt.Errorf("failed to create platform client: %w", err)
	}

	remoteURL, err := repo.GetRemoteURL("origin")
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %w", err)
	}

	if err := provider.Initialize(remoteURL); err != nil {
		return fmt.Errorf("failed to initialize %s client: %w", provider.PlatformName(), err)
	}

	availableLabels, err := provider.ListLabels()
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	fmt.Printf("Available labels for %s:%s:\n", provider.PlatformName(), remoteURL)
	for _, label := range availableLabels {
		fmt.Printf("- %s\n", label.Name)
	}
	fmt.Printf("\nTotal: %d labels\n", len(availableLabels))
	return nil
}

func selectLabels(
	provider platform.Provider, useManualSelection bool, manualLabels string, title string,
) ([]string, error) {
	availableLabels, err := provider.ListLabels()
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	if useManualSelection {
		log.Debug("Using manual label selection via --labels flag")
		return validateManualLabels(availableLabels, manualLabels)
	}

	// Automatic selection based on conventional commit type
	log.Debug("Using automatic label selection from commit type")
	availableNames := make([]string, len(availableLabels))
	for i, label := range availableLabels {
		availableNames[i] = label.Name
	}

	selected := autolabels.AutoSelectLabels(title, availableNames)
	if len(selected) > 0 {
		log.Infof("Auto-selected labels: %v", selected)
	} else {
		log.Debug("No labels matched commit type, proceeding without labels")
	}

	return selected, nil
}

func createMR(
	provider platform.Provider,
	currentBranch, mainBranch, title, body string,
	selectedLabels []string,
	squash bool,
) (*platform.MergeRequest, error) {
	log.IncreasePadding()
	log.Infof("Creating %s merge/pull request...", provider.PlatformName())

	mr, err := provider.Create(platform.CreateParams{
		SourceBranch: currentBranch,
		TargetBranch: mainBranch,
		Title:        title,
		Body:         body,
		Labels:       selectedLabels,
		Squash:       squash,
	})
	if err != nil {
		if errors.Is(err, platform.ErrAlreadyExists) {
			log.Warnf("Merge/pull request already exists for branch: %s", currentBranch)
			existingMR, fetchErr := provider.GetByBranch(currentBranch, mainBranch)
			if fetchErr != nil {
				return nil, fmt.Errorf("failed to fetch existing merge/pull request: %w", fetchErr)
			}
			log.Infof("Using existing merge/pull request: %s", existingMR.WebURL)
			log.DecreasePadding()
			return existingMR, nil
		}
		log.DecreasePadding()
		return nil, fmt.Errorf("failed to create merge/pull request: %w", err)
	}

	log.Infof("Merge/pull request created: %s", mr.WebURL)
	log.DecreasePadding()
	return mr, nil
}

func waitAndMerge(
	cmd *cobra.Command,
	provider platform.Provider,
	mr *platform.MergeRequest,
	squash bool,
	commitTitle string,
) error {
	time.Sleep(pipelineStartupDelay)

	timeout, err := getPipelineTimeout(cmd, provider.PipelineTimeout())
	if err != nil {
		return err
	}

	status, err := provider.WaitForPipeline(timeout)
	if err != nil {
		return fmt.Errorf("failed to wait for pipeline: %w", err)
	}

	if status != "success" && status != "" {
		return fmt.Errorf("%w with status: %s", errPipelineFailed, status)
	}

	log.Infof("Merging %s merge/pull request...", provider.PlatformName())
	log.IncreasePadding()

	log.Info("Approving merge/pull request...")
	if err := provider.Approve(mr.ID); err != nil {
		log.Warnf("Failed to approve merge/pull request: %v", err)
	}

	if err := provider.Merge(platform.MergeParams{
		MRID:         mr.ID,
		Squash:       squash,
		CommitTitle:  commitTitle,
		SourceBranch: mr.SourceBranch,
	}); err != nil {
		log.DecreasePadding()
		return fmt.Errorf("failed to merge: %w", err)
	}

	log.Info("Merge/pull request merged successfully")
	log.DecreasePadding()
	return nil
}

func validateManualLabels(availableLabels []platform.Label, requestedLabels string) ([]string, error) {
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
	availableMap := make(map[string]bool, len(availableLabels))
	for _, label := range availableLabels {
		availableMap[label.Name] = true
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

func cleanup(ctx context.Context, repo *git.Repository, mainBranch, currentBranch string) error {
	log.Info("Cleanup...")
	log.IncreasePadding()
	defer log.DecreasePadding()

	log.Infof("Switching to main branch: %s", mainBranch)
	report := repo.Cleanup(ctx, mainBranch, currentBranch)

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
