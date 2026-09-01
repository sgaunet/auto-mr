package security_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sgaunet/auto-mr/internal/security"
	"github.com/sgaunet/bullets"
)

// These two helpers are the credential-redaction surface used by the git auth path,
// so their whole purpose is that a token or a home directory never reaches the log.

func debugLogger() (*bullets.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := bullets.New(&buf)
	logger.SetLevel(bullets.DebugLevel)
	return logger, &buf
}

func TestDebugAuth_RedactsTokenValues(t *testing.T) {
	const token = "glpat-canarytoken1234567890"

	logger, buf := debugLogger()
	security.DebugAuth(logger, "GitLab", map[string]string{
		"method": "token",
		"token":  token,
		"url":    "https://gitlab.com/org/repo.git",
	})

	out := buf.String()
	if out == "" {
		t.Fatal("DebugAuth wrote nothing; the assertions below would be vacuous")
	}
	if strings.Contains(out, token) {
		t.Errorf("token leaked into the log:\n%s", out)
	}
	if !strings.Contains(out, "GitLab") {
		t.Errorf("log does not name the auth type:\n%s", out)
	}
}

func TestDebugAuth_NilLoggerIsSafe(_ *testing.T) {
	// The git package resolves auth before a logger is attached, so nil must be
	// tolerated rather than panicking.
	security.DebugAuth(nil, "GitLab", map[string]string{"token": "glpat-canary"})
}

func TestDebugSSHKey_MasksHomeDirectory(t *testing.T) {
	tests := []struct {
		name    string
		success bool
	}{
		{name: "configured", success: true},
		{name: "attempted", success: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := debugLogger()
			security.DebugSSHKey(logger, "/Users/someone/.ssh/id_ed25519", tt.success)

			out := buf.String()
			if out == "" {
				t.Fatal("DebugSSHKey wrote nothing")
			}
			if strings.Contains(out, "/Users/someone") {
				t.Errorf("absolute home path leaked into the log:\n%s", out)
			}
			if !strings.Contains(out, "~/.ssh/") {
				t.Errorf("log does not show the masked key path:\n%s", out)
			}
		})
	}
}

func TestDebugSSHKey_NilLoggerIsSafe(_ *testing.T) {
	security.DebugSSHKey(nil, "/Users/someone/.ssh/id_ed25519", true)
}
