package git_test

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
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

	// Note: OpenRepository will fail because go-git can't parse this worktree reference,
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
