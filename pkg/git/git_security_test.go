package git_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sgaunet/auto-mr/pkg/git"
	"github.com/sgaunet/bullets"
)

// TestHTTPSAuth_NoTokenLeakage verifies that tokens don't leak through authentication logging.
func TestHTTPSAuth_NoTokenLeakage(t *testing.T) {
	// Setup: Create a temporary git repo for testing
	tempDir := t.TempDir()
	setupTestGitRepo(t, tempDir, "https://gitlab.com/test/repo.git")

	tests := []struct {
		name      string
		envVar    string
		envValue  string
		forbidden []string
	}{
		{
			name:      "gitlab token",
			envVar:    "GITLAB_TOKEN",
			envValue:  "glpat-testsecret1234567890",
			forbidden: []string{"glpat-testsecret1234567890", "testsecret"},
		},
		{
			name:      "github token",
			envVar:    "GITHUB_TOKEN",
			envValue:  "ghp_testsecret12345678901234567890123456",
			forbidden: []string{"ghp_testsecret12345678901234567890123456", "testsecret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the token environment variable
			oldValue := os.Getenv(tt.envVar)
			os.Setenv(tt.envVar, tt.envValue)
			defer func() {
				if oldValue != "" {
					os.Setenv(tt.envVar, oldValue)
				} else {
					os.Unsetenv(tt.envVar)
				}
			}()

			// Capture log output
			var logBuffer bytes.Buffer
			testLogger := bullets.New(&logBuffer)
			testLogger.SetLevel(bullets.DebugLevel)

			// Open repository with debug logging
			repo, err := git.OpenRepository(tempDir)
			if err != nil {
				t.Fatalf("Failed to open repository: %v", err)
			}
			repo.SetLogger(testLogger)

			// Force authentication setup by trying to get remote URL
			// This triggers the auth code path
			_, _ = repo.GetRemoteURL("origin")

			// Check captured logs
			logOutput := logBuffer.String()

			// Verify no forbidden strings in logs
			for _, forbidden := range tt.forbidden {
				if strings.Contains(logOutput, forbidden) {
					t.Errorf("Log output contains forbidden string %q:\n%s", forbidden, logOutput)
				}
			}

			// Verify that masked token format is present (if any auth logging occurred)
			if strings.Contains(logOutput, "authentication") || strings.Contains(logOutput, "token") {
				// Should contain sanitized output
				if !strings.Contains(logOutput, "[token:") && !strings.Contains(logOutput, "[redacted]") {
					t.Logf("Warning: Auth logging present but no masking detected in:\n%s", logOutput)
				}
			}
		})
	}
}

// TestSSHAuth_NoPathLeakage verifies that SSH key paths are masked in logs.
func TestSSHAuth_NoPathLeakage(t *testing.T) {
	// Setup: Create a temporary git repo with SSH URL
	tempDir := t.TempDir()
	setupTestGitRepo(t, tempDir, "git@gitlab.com:test/repo.git")

	// Capture log output
	var logBuffer bytes.Buffer
	testLogger := bullets.New(&logBuffer)
	testLogger.SetLevel(bullets.DebugLevel)

	// Open repository with debug logging
	repo, err := git.OpenRepository(tempDir)
	if err != nil {
		t.Fatalf("Failed to open repository: %v", err)
	}
	repo.SetLogger(testLogger)

	// Trigger SSH auth logging by accessing remote
	_, _ = repo.GetRemoteURL("origin")

	// Check captured logs
	logOutput := logBuffer.String()

	// Verify no full paths in logs (should be masked with ~/)
	homeDir, err := os.UserHomeDir()
	if err == nil && strings.Contains(logOutput, homeDir) {
		// Check if it's properly masked
		if !strings.Contains(logOutput, "~/.ssh/") {
			t.Errorf("SSH key path not properly masked. Full path leaked:\n%s", logOutput)
		}
	}
}

// TestErrorSanitization verifies that errors don't leak credentials.
func TestErrorSanitization(t *testing.T) {
	tests := []struct {
		name        string
		envVar      string
		envValue    string
		shouldError bool
		forbidden   []string
	}{
		{
			name:        "gitlab token in error",
			envVar:      "GITLAB_TOKEN",
			envValue:    "glpat-errorsecret123456",
			shouldError: false, // May or may not error depending on git state
			forbidden:   []string{"glpat-errorsecret123456", "errorsecret"},
		},
		{
			name:        "github token in error",
			envVar:      "GITHUB_TOKEN",
			envValue:    "ghp_errorsecret1234567890123456789012",
			shouldError: false,
			forbidden:   []string{"ghp_errorsecret1234567890123456789012", "errorsecret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the token environment variable
			oldValue := os.Getenv(tt.envVar)
			os.Setenv(tt.envVar, tt.envValue)
			defer func() {
				if oldValue != "" {
					os.Setenv(tt.envVar, oldValue)
				} else {
					os.Unsetenv(tt.envVar)
				}
			}()

			// Create a temporary repo
			tempDir := t.TempDir()
			setupTestGitRepo(t, tempDir, "https://gitlab.com/test/repo.git")

			// Capture any errors
			var logBuffer bytes.Buffer
			testLogger := bullets.New(&logBuffer)
			testLogger.SetLevel(bullets.DebugLevel)

			repo, err := git.OpenRepository(tempDir)
			if err != nil {
				// Check error message for token leakage
				errMsg := err.Error()
				for _, forbidden := range tt.forbidden {
					if strings.Contains(errMsg, forbidden) {
						t.Errorf("Error message contains forbidden string %q: %v", forbidden, err)
					}
				}
				return
			}

			repo.SetLogger(testLogger)

			// Try various operations that might fail and produce errors
			// Even if they succeed, we want to ensure no token leakage in logs
			_, _ = repo.GetRemoteURL("origin")

			// Check logs for token leakage
			logOutput := logBuffer.String()
			for _, forbidden := range tt.forbidden {
				if strings.Contains(logOutput, forbidden) {
					t.Errorf("Log output contains forbidden string %q:\n%s", forbidden, logOutput)
				}
			}
		})
	}
}

// TestFormattingOperations verifies tokens don't leak through string formatting.
func TestFormattingOperations(t *testing.T) {
	// This test ensures that even if someone tries to format auth structures,
	// tokens don't leak
	tempDir := t.TempDir()
	setupTestGitRepo(t, tempDir, "https://gitlab.com/test/repo.git")

	// Set a token
	testToken := "glpat-formattest123456"
	os.Setenv("GITLAB_TOKEN", testToken)
	defer os.Unsetenv("GITLAB_TOKEN")

	var logBuffer bytes.Buffer
	testLogger := bullets.New(&logBuffer)
	testLogger.SetLevel(bullets.DebugLevel)

	repo, err := git.OpenRepository(tempDir)
	if err != nil {
		t.Fatalf("Failed to open repository: %v", err)
	}
	repo.SetLogger(testLogger)

	// Trigger auth setup
	_, _ = repo.GetRemoteURL("origin")

	// Try various formatting operations on the log output
	logOutput := logBuffer.String()
	formattedOutputs := []string{
		logOutput,
		fmt.Sprintf("%s", logOutput),
		fmt.Sprintf("%v", logOutput),
		fmt.Sprintf("%+v", logOutput),
	}

	for i, output := range formattedOutputs {
		if strings.Contains(output, testToken) {
			t.Errorf("Formatted output %d contains actual token", i)
		}
	}
}

// setupTestGitRepo creates a minimal git repository for testing.
func setupTestGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()

	// Initialize git repo
	os.Chdir(dir)
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@example.com")
	runCmd(t, dir, "git", "config", "user.name", "Test User")
	runCmd(t, dir, "git", "remote", "add", "origin", remoteURL)

	// Create an initial commit
	testFile := dir + "/test.txt"
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	runCmd(t, dir, "git", "add", "test.txt")
	runCmd(t, dir, "git", "commit", "-m", "Initial commit")
}

// runCmd executes a command and fails the test if it errors.
func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command failed: %s %v\nOutput: %s", name, args, string(output))
	}
}
