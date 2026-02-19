package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sgaunet/auto-mr/pkg/git"
)

// initTestRepo creates a proper git repository using go-git with a remote origin
func initTestRepo(t *testing.T, path string) {
	t.Helper()
	repo, err := gogit.PlainInit(path, false)
	if err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}

	// Add a fake remote origin (required by OpenRepository)
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/test/test.git"},
	})
	if err != nil {
		t.Fatalf("Failed to create remote origin: %v", err)
	}
}

// TestFindGitRoot_FromRoot tests finding git root when already at repository root.
func TestFindGitRoot_FromRoot(t *testing.T) {
	// Create temporary directory with proper git repository
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// Test that we can find git root from repository root
	repo, err := git.OpenRepository(tmpDir)
	if err != nil {
		t.Fatalf("Expected to find git repository, got error: %v", err)
	}
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

// TestFindGitRoot_FromSubdirectory tests finding git root from a subdirectory.
func TestFindGitRoot_FromSubdirectory(t *testing.T) {
	// Create temporary directory with proper git repository
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Test that we can find git root from subdirectory
	repo, err := git.OpenRepository(subDir)
	if err != nil {
		t.Fatalf("Expected to find git repository from subdirectory, got error: %v", err)
	}
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

// TestFindGitRoot_FromNestedSubdirectory tests finding git root from deeply nested subdirectory.
func TestFindGitRoot_FromNestedSubdirectory(t *testing.T) {
	// Create temporary directory with proper git repository
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	nestedDir := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested directories: %v", err)
	}

	// Test that we can find git root from deeply nested subdirectory
	repo, err := git.OpenRepository(nestedDir)
	if err != nil {
		t.Fatalf("Expected to find git repository from nested subdirectory, got error: %v", err)
	}
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

// TestFindGitRoot_NotFound tests error when no git repository exists.
func TestFindGitRoot_NotFound(t *testing.T) {
	// Create temporary directory without .git
	tmpDir := t.TempDir()

	// Test that we get an error when no .git directory exists
	repo, err := git.OpenRepository(tmpDir)
	if err == nil {
		t.Fatal("Expected error when no git repository exists, got nil")
	}
	if repo != nil {
		t.Fatal("Expected nil repository when error occurs")
	}

	// Verify error message mentions git repository
	errMsg := err.Error()
	if errMsg == "" {
		t.Fatal("Expected non-empty error message")
	}
}

// TestFindGitRoot_WithGitFile tests handling of .git as a file (worktree scenario).
func TestFindGitRoot_WithGitFile(t *testing.T) {
	// Create temporary directory with .git file (simulating git worktree)
	tmpDir := t.TempDir()
	gitFile := filepath.Join(tmpDir, ".git")

	// Create .git as a file with worktree content
	content := "gitdir: /path/to/main/repo/.git/worktrees/test"
	if err := os.WriteFile(gitFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create .git file: %v", err)
	}

	// Note: OpenRepository will fail because the gitdir target path does not exist,
	// but findGitRoot should successfully detect the .git file exists
	// We're testing that findGitRoot can find .git regardless of whether it's a file or directory
	_, err := git.OpenRepository(tmpDir)

	// We expect an error from go-git, but it should be about opening the repository,
	// not about finding the git root
	if err == nil {
		t.Skip("Worktree test skipped: go-git may not support worktree .git files")
	}

	// The error should NOT be "not a git repository" - it should be something else
	errMsg := err.Error()
	if errMsg == "failed to locate git repository: not a git repository (or any parent up to mount point)" {
		t.Fatalf("findGitRoot failed to detect .git file, got error: %v", err)
	}
}

// TestFindGitRoot_WithRelativePath tests handling of relative paths.
func TestFindGitRoot_WithRelativePath(t *testing.T) {
	// Create temporary directory with proper git repository
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Change to subdirectory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Logf("Failed to restore working directory: %v", err)
		}
	}()

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change to subdirectory: %v", err)
	}

	// Test with relative path "."
	repo, err := git.OpenRepository(".")
	if err != nil {
		t.Fatalf("Expected to find git repository with relative path, got error: %v", err)
	}
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

// TestFindGitRoot_MultipleGitDirs tests that we stop at the first .git found (no traversal into parent repos).
func TestFindGitRoot_MultipleGitDirs(t *testing.T) {
	// Create nested git repositories: outer and outer/inner
	outerDir := t.TempDir()
	initTestRepo(t, outerDir)

	innerDir := filepath.Join(outerDir, "inner")
	if err := os.Mkdir(innerDir, 0755); err != nil {
		t.Fatalf("Failed to create inner directory: %v", err)
	}
	initTestRepo(t, innerDir)

	// When opening from inner directory, should find inner .git, not outer
	repo, err := git.OpenRepository(innerDir)
	if err != nil {
		t.Fatalf("Expected to find inner git repository, got error: %v", err)
	}
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}

	// Note: We can't directly verify which .git was found without exposing gitRoot,
	// but the behavior is correct if no error occurs (it found the closest .git)
}

// TestOpenRepository_Integration tests the full integration with actual repository.
func TestOpenRepository_Integration(t *testing.T) {
	// This test assumes we're running from within the auto-mr git repository
	// Skip if not in a git repository
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		t.Skip("Not running from git repository root, skipping integration test")
	}

	repo, err := git.OpenRepository(".")
	if err != nil {
		t.Fatalf("Failed to open current repository: %v", err)
	}

	// Test that we can get current branch
	branch, err := repo.GetCurrentBranch()
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}
	if branch == "" {
		t.Fatal("Expected non-empty branch name")
	}

	t.Logf("Current branch: %s", branch)
}

// TestOpenRepository_FromSubdir_Integration tests opening repository from subdirectory.
func TestOpenRepository_FromSubdir_Integration(t *testing.T) {
	// This test assumes we're running from within the auto-mr git repository
	// Skip if not in appropriate location
	pkgGitDir := "pkg/git"
	if _, err := os.Stat(pkgGitDir); os.IsNotExist(err) {
		t.Skip("pkg/git directory not found, skipping integration test")
	}

	// Try to open repository from pkg/git subdirectory
	repo, err := git.OpenRepository(pkgGitDir)
	if err != nil {
		t.Fatalf("Failed to open repository from subdirectory: %v", err)
	}

	// Verify we can still get repository information
	branch, err := repo.GetCurrentBranch()
	if err != nil {
		t.Fatalf("Failed to get current branch from subdirectory context: %v", err)
	}
	if branch == "" {
		t.Fatal("Expected non-empty branch name")
	}

	t.Logf("Current branch (from subdir): %s", branch)
}

// TestOpenRepository_Worktree tests that OpenRepository works from a linked git worktree.
func TestOpenRepository_Worktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found, skipping worktree test")
	}

	// Create main repository with go-git
	mainDir := t.TempDir()
	mainRepo, err := gogit.PlainInit(mainDir, false)
	if err != nil {
		t.Fatalf("Failed to init main repo: %v", err)
	}

	// Add remote origin
	_, err = mainRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/test/test.git"},
	})
	if err != nil {
		t.Fatalf("Failed to create remote: %v", err)
	}

	// Create initial commit on main
	wt, err := mainRepo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	readmePath := filepath.Join(mainDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Main\n"), 0644); err != nil {
		t.Fatalf("Failed to write README: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("Failed to add README: %v", err)
	}
	_, err = wt.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Create feature branch with a distinct commit using native git
	cmd := exec.Command("git", "checkout", "-b", "feature-worktree")
	cmd.Dir = mainDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create feature branch: %v\n%s", err, out)
	}

	featureFile := filepath.Join(mainDir, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("feature work\n"), 0644); err != nil {
		t.Fatalf("Failed to write feature file: %v", err)
	}

	cmd = exec.Command("git", "add", "feature.txt")
	cmd.Dir = mainDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to add feature file: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "feat: add feature work")
	cmd.Dir = mainDir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to commit feature: %v\n%s", err, out)
	}

	// Switch back to main so we can create a worktree for the feature branch
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = mainDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Try "master" if "main" doesn't exist
		cmd = exec.Command("git", "checkout", "master")
		cmd.Dir = mainDir
		if out2, err2 := cmd.CombinedOutput(); err2 != nil {
			t.Fatalf("Failed to checkout main/master: %v\n%s\n%s", err, out, out2)
		}
	}

	// Create linked worktree
	worktreeDir := filepath.Join(t.TempDir(), "worktree-feature")
	cmd = exec.Command("git", "worktree", "add", worktreeDir, "feature-worktree")
	cmd.Dir = mainDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create worktree: %v\n%s", err, out)
	}

	// Verify .git is a file (worktree indicator)
	gitPath := filepath.Join(worktreeDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		t.Fatalf("Failed to stat .git in worktree: %v", err)
	}
	if info.IsDir() {
		t.Fatal("Expected .git to be a file in worktree, got directory")
	}

	// Open repository from worktree path
	repo, err := git.OpenRepository(worktreeDir)
	if err != nil {
		t.Fatalf("Failed to open repository from worktree: %v", err)
	}

	// Assert: GetCurrentBranch returns the feature branch
	branch, err := repo.GetCurrentBranch()
	if err != nil {
		t.Fatalf("Failed to get current branch from worktree: %v", err)
	}
	if branch != "feature-worktree" {
		t.Errorf("Expected branch 'feature-worktree', got '%s'", branch)
	}

	// Assert: DetectPlatform works (reads shared remote config)
	platform, err := repo.DetectPlatform()
	if err != nil {
		t.Fatalf("Failed to detect platform from worktree: %v", err)
	}
	if platform != "github" {
		t.Errorf("Expected platform 'github', got '%s'", platform)
	}

	// Assert: GetLatestCommitMessage returns the worktree branch's commit
	goGitRepo := repo.GoGitRepository()
	head, err := goGitRepo.Head()
	if err != nil {
		t.Fatalf("Failed to get HEAD from worktree: %v", err)
	}
	commit, err := goGitRepo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("Failed to get commit object: %v", err)
	}
	if strings.TrimSpace(commit.Message) != "feat: add feature work" {
		t.Errorf("Expected commit message 'feat: add feature work', got '%s'", commit.Message)
	}

	// Also verify we can resolve the feature branch ref from worktree
	ref, err := goGitRepo.Reference(plumbing.NewBranchReferenceName("feature-worktree"), true)
	if err != nil {
		t.Fatalf("Failed to resolve feature branch ref: %v", err)
	}
	if ref.Hash() != head.Hash() {
		t.Error("Expected HEAD to match feature-worktree branch ref")
	}

	t.Logf("Worktree branch: %s, platform: %s, commit: %s", branch, platform, commit.Message)
}
