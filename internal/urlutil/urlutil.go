// Package urlutil provides URL parsing utilities for extracting path components
// from git remote URLs.
//
// It handles three URL formats:
//   - HTTPS: https://github.com/owner/repo
//   - SSH colon: git@github.com:owner/repo
//   - SSH protocol: ssh://git@github.com/owner/repo
//
// The .git suffix should be removed by the caller before calling [ExtractPathComponents].
package urlutil

import "strings"

const (
	// minColonParts is the minimum number of parts expected when splitting SSH colon format URLs.
	// SSH colon format: git@host:path splits into ["git@host", "path"].
	minColonParts = 2
)

// ExtractPathComponents extracts the last N path components from a git remote URL.
// It handles multiple URL formats:
//   - HTTPS: https://github.com/owner/repo (expects .git suffix already removed)
//   - SSH colon: git@github.com:owner/repo (expects .git suffix already removed)
//   - SSH protocol: ssh://git@github.com/owner/repo (expects .git suffix already removed)
//
// The componentCount parameter specifies how many path components to extract.
// Returns empty string if the URL doesn't contain enough components.
//
// Note: The caller should trim the .git suffix before calling this function,
// as done in the GitHub and GitLab packages.
//
// Examples:
//
//	ExtractPathComponents("git@github.com:owner/repo", 2) → "owner/repo"
//	ExtractPathComponents("https://gitlab.com/group/subgroup/project", 2) → "subgroup/project"
//	ExtractPathComponents("https://gitlab.com/group/subgroup/project", 3) → "group/subgroup/project"
func ExtractPathComponents(url string, componentCount int) string {
	// Handle ssh:// protocol format separately from git@ colon format
	if strings.HasPrefix(url, "ssh://git@") {
		// SSH protocol format: ssh://git@host/path
		// Use slash-based parsing
		parts := strings.Split(url, "/")
		if len(parts) >= componentCount {
			return strings.Join(parts[len(parts)-componentCount:], "/")
		}
		return ""
	}

	if strings.HasPrefix(url, "git@") {
		// SSH colon format: git@host:path
		parts := strings.Split(url, ":")
		if len(parts) >= minColonParts {
			// Return everything after the last colon
			return parts[len(parts)-1]
		}
		return ""
	}

	// HTTPS format
	parts := strings.Split(url, "/")
	if len(parts) >= componentCount {
		return strings.Join(parts[len(parts)-componentCount:], "/")
	}
	return ""
}
