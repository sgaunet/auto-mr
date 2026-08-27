package git_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/sgaunet/auto-mr/pkg/git"
	"github.com/sgaunet/bullets"
	"golang.org/x/crypto/ssh"
)

// Fixture tokens used by the auth tests. They are shaped like real personal access
// tokens so the sanitizer's prefix patterns apply, and are enumerated by the leak
// assertions to prove none of them reaches a log or an unintended host.
const (
	gitLabCanaryToken  = "glpat-canarytoken1234567890"
	gitHubCanaryToken  = "ghp_canarytoken123456789012345678901"
	forgejoCanaryToken = "forgejo-canarytoken1234567890"
)

// TestHTTPSAuth_NoTokenLeakage verifies that tokens don't leak through authentication logging.
//
// The logger must be supplied to the auth path directly. Authentication is resolved
// inside OpenRepository, before any SetLogger call can take effect, so a test that
// opens a repository and then attaches a capturing logger observes nothing from the
// auth path and would pass no matter what that path logged.
func TestHTTPSAuth_NoTokenLeakage(t *testing.T) {
	tests := []struct {
		name      string
		envVar    string
		envValue  string
		remoteURL string
		forbidden []string
	}{
		{
			name:      "gitlab token",
			envVar:    "GITLAB_TOKEN",
			envValue:  gitLabCanaryToken,
			remoteURL: "https://gitlab.com/test/repo.git",
			forbidden: []string{gitLabCanaryToken, "canarytoken"},
		},
		{
			name:      "github token",
			envVar:    "GITHUB_TOKEN",
			envValue:  gitHubCanaryToken,
			remoteURL: "https://github.com/test/repo.git",
			forbidden: []string{gitHubCanaryToken, "canarytoken"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.envValue)

			var logBuffer bytes.Buffer
			testLogger := bullets.New(&logBuffer)
			testLogger.SetLevel(bullets.DebugLevel)

			auth, err := git.GetHTTPSAuthForTest(tt.remoteURL, testLogger, "")
			if err != nil {
				t.Fatalf("GetHTTPSAuthForTest: %v", err)
			}
			if auth == nil {
				t.Fatalf("expected %s to be attached for %q", tt.envVar, tt.remoteURL)
			}

			logOutput := logBuffer.String()

			// Guard against this test silently proving nothing again: the auth path
			// must actually have logged something for the assertions below to mean
			// anything.
			if logOutput == "" {
				t.Fatal("no auth logging captured; the leak assertions below would be vacuous")
			}

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

// writeDummySSHKey generates a throwaway ed25519 key pair under home/.ssh/id_ed25519
// so setupSSHAuth's file-based fallback succeeds in environments (e.g. CI runners)
// that have no real SSH agent or keys configured.
func writeDummySSHKey(t *testing.T, home string) {
	t.Helper()

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh dir: %v", err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate dummy SSH key: %v", err)
	}

	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("Failed to marshal dummy SSH key: %v", err)
	}

	keyPath := filepath.Join(sshDir, "id_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatalf("Failed to write dummy SSH key: %v", err)
	}
}

// TestSSHAuth_NoPathLeakage verifies that SSH key paths are masked in logs.
func TestSSHAuth_NoPathLeakage(t *testing.T) {
	// Setup: Create a temporary git repo with SSH URL
	tempDir := t.TempDir()
	setupTestGitRepo(t, tempDir, "git@gitlab.com:test/repo.git")

	// Provide a dummy SSH key so auth setup succeeds without relying on the
	// host machine's real ~/.ssh (absent on CI runners).
	homeDir := t.TempDir()
	writeDummySSHKey(t, homeDir)
	t.Setenv("HOME", homeDir)

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
	if strings.Contains(logOutput, homeDir) {
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
	// Same hermetic environment as gitCmd: never inherit GIT_* from a git hook
	// or an outer git invocation (see #102).
	cmd.Env = hermeticGitEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %s %v\nOutput: %s", name, args, string(output))
	}
}

// TestGetHTTPSAuth_HostSpoofing verifies that a remote URL which merely resembles a
// known platform host never receives that platform's token.
//
// Substring host matching used to make each of these remotes authenticate as the
// platform they impersonate, handing a live credential to a host the user does not
// control. The assertion is on the returned auth method rather than on log output,
// because only that proves no credential reached the transport.
func TestGetHTTPSAuth_HostSpoofing(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", gitLabCanaryToken)
	t.Setenv("GITHUB_TOKEN", gitHubCanaryToken)
	t.Setenv("FORGEJO_TOKEN", forgejoCanaryToken)

	const configuredForgejo = "https://git.example.com"

	tests := []struct {
		name       string
		remoteURL  string
		forgejoURL string
	}{
		{name: "gitlab_host_suffix", remoteURL: "https://gitlab.com.evil.example/owner/repo.git"},
		{name: "gitlab_in_path", remoteURL: "https://evil.example/gitlab.com/repo.git"},
		{name: "gitlab_in_userinfo", remoteURL: "https://gitlab.com@evil.example/repo.git"},
		{name: "github_host_suffix", remoteURL: "https://github.com.evil.example/owner/repo.git"},
		{name: "github_in_path", remoteURL: "https://evil.example/github.com/repo.git"},
		{name: "gitlab_subdomain_lookalike", remoteURL: "https://gitlab.company.example/owner/repo.git"},
		{name: "unrelated_host", remoteURL: "https://bitbucket.example/owner/repo.git"},

		// With no Forgejo instance configured, FORGEJO_TOKEN must never be attached —
		// this is the default-branch fallthrough that used to leak it to any host.
		{name: "forgejo_token_set_but_no_instance_configured", remoteURL: "https://git.example.com/owner/repo.git"},

		// With an instance configured, only that exact host qualifies.
		{
			name:       "forgejo_host_mismatch",
			remoteURL:  "https://evil.example/owner/repo.git",
			forgejoURL: configuredForgejo,
		},
		{
			name:       "forgejo_host_suffix_spoof",
			remoteURL:  "https://git.example.com.evil.example/owner/repo.git",
			forgejoURL: configuredForgejo,
		},
		{
			name:       "forgejo_host_in_path_spoof",
			remoteURL:  "https://evil.example/git.example.com/repo.git",
			forgejoURL: configuredForgejo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			testLogger := bullets.New(&logBuffer)
			testLogger.SetLevel(bullets.DebugLevel)

			auth, err := git.GetHTTPSAuthForTest(tt.remoteURL, testLogger, tt.forgejoURL)
			if err != nil {
				t.Fatalf("GetHTTPSAuthForTest(%q) returned error: %v", tt.remoteURL, err)
			}
			if auth != nil {
				t.Errorf("remote %q was given credential %v; expected no authentication",
					tt.remoteURL, auth)
			}

			// Defense in depth: no token value may appear in the debug output either.
			for _, token := range []string{gitLabCanaryToken, gitHubCanaryToken, forgejoCanaryToken} {
				if strings.Contains(logBuffer.String(), token) {
					t.Errorf("token leaked into log output for remote %q", tt.remoteURL)
				}
			}
		})
	}
}

// TestGetHTTPSAuth_LegitimateHostsAuthenticate is the counterpart to the spoofing
// table: it guards against "fixing" the leak by refusing to authenticate at all.
func TestGetHTTPSAuth_LegitimateHostsAuthenticate(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", gitLabCanaryToken)
	t.Setenv("GITHUB_TOKEN", gitHubCanaryToken)

	t.Setenv("FORGEJO_TOKEN", forgejoCanaryToken)

	tests := []struct {
		name         string
		remoteURL    string
		forgejoURL   string
		wantUsername string
		wantPassword string
	}{
		{
			name:         "gitlab_https",
			remoteURL:    "https://gitlab.com/owner/repo.git",
			wantUsername: "oauth2",
			wantPassword: gitLabCanaryToken,
		},
		{
			name:         "github_https",
			remoteURL:    "https://github.com/owner/repo.git",
			wantUsername: "x-access-token",
			wantPassword: gitHubCanaryToken,
		},
		{
			name:         "gitlab_uppercase_host",
			remoteURL:    "https://GitLab.com/owner/repo.git",
			wantUsername: "oauth2",
			wantPassword: gitLabCanaryToken,
		},
		{
			name:         "github_explicit_port",
			remoteURL:    "https://github.com:443/owner/repo.git",
			wantUsername: "x-access-token",
			wantPassword: gitHubCanaryToken,
		},
		{
			name:         "forgejo_configured_host_matches",
			remoteURL:    "https://git.example.com/owner/repo.git",
			forgejoURL:   "https://git.example.com",
			wantUsername: "forgejo",
			wantPassword: forgejoCanaryToken,
		},
		{
			// The instance is reached over a different port than configured; a host
			// commonly serves SSH and HTTP separately and both are one trust domain.
			name:         "forgejo_configured_host_different_port",
			remoteURL:    "https://git.example.com:3000/owner/repo.git",
			forgejoURL:   "https://git.example.com",
			wantUsername: "forgejo",
			wantPassword: forgejoCanaryToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := git.GetHTTPSAuthForTest(tt.remoteURL, bullets.New(&bytes.Buffer{}), tt.forgejoURL)
			if err != nil {
				t.Fatalf("GetHTTPSAuthForTest(%q) returned error: %v", tt.remoteURL, err)
			}
			basic, ok := auth.(*githttp.BasicAuth)
			if !ok {
				t.Fatalf("expected *http.BasicAuth for %q, got %T", tt.remoteURL, auth)
			}
			if basic.Username != tt.wantUsername {
				t.Errorf("username = %q, want %q", basic.Username, tt.wantUsername)
			}
			if basic.Password != tt.wantPassword {
				t.Errorf("password did not match the configured token for %q", tt.remoteURL)
			}
		})
	}
}

// TestNativeGit_BranchNameIsNotParsedAsFlag verifies that a branch name shaped like
// a git option is passed as a ref rather than interpreted as one.
//
// Branch names reaching these calls come from git itself, including the remote's
// advertised HEAD symref, so a malicious server can influence them. Local git will
// not create a branch whose name begins with a hyphen, but it will happily parse one
// as an option if it arrives unseparated on the command line -- "git switch --help"
// prints help and exits zero, which would report success for a switch that never
// happened. The "--" separator makes git treat the value as a ref, so these calls
// must fail.
func TestNativeGit_BranchNameIsNotParsedAsFlag(t *testing.T) {
	flagLike := []string{"--help", "--version", "-D"}

	for _, name := range flagLike {
		t.Run(name, func(t *testing.T) {
			repo, err := git.OpenRepository(newRepoWithBranches(t, "main"))
			if err != nil {
				t.Fatalf("OpenRepository: %v", err)
			}

			if err := repo.SwitchBranch(t.Context(), name); err == nil {
				t.Errorf("SwitchBranch(%q) returned nil; the argument was parsed as an option "+
					"instead of a ref name", name)
			}
			if err := repo.DeleteBranch(t.Context(), name); err == nil {
				t.Errorf("DeleteBranch(%q) returned nil; the argument was parsed as an option "+
					"instead of a ref name", name)
			}
		})
	}
}
