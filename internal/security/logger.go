package security

import (
	"fmt"

	"github.com/sgaunet/bullets"
)

// DebugAuth logs authentication information safely.
// All details are sanitized before logging to prevent token leakage.
//
// Example:
//
//	DebugAuth(logger, "GitLab", map[string]string{
//	    "method": "token",
//	    "url": "https://gitlab.com/org/repo.git",
//	})
func DebugAuth(logger *bullets.Logger, authType string, details map[string]string) {
	if logger == nil {
		return
	}

	// Convert to any map for sanitization
	detailsInterface := make(map[string]any, len(details))
	for k, v := range details {
		detailsInterface[k] = v
	}

	sanitized := SanitizeMap(detailsInterface)
	logger.Debug(fmt.Sprintf("Using %s authentication: %v", authType, sanitized))
}

// DebugSSHKey logs SSH key usage safely with masked paths.
//
// Example:
//
//	DebugSSHKey(logger, "/Users/john/.ssh/id_ed25519", true)
//	// Logs: "SSH authentication configured with key: ~/.ssh/id_ed25519"
func DebugSSHKey(logger *bullets.Logger, keyFile string, success bool) {
	if logger == nil {
		return
	}

	maskedPath := MaskSSHKeyPath(keyFile)

	if success {
		logger.Debug("SSH authentication configured with key: " + maskedPath)
	} else {
		logger.Debug("Trying SSH key: " + maskedPath)
	}
}
