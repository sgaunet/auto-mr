// Package urlutil provides URL parsing utilities for git remote URLs: extracting
// path components with [ExtractPathComponents] and the bare hostname with [Host].
//
// It handles three URL formats:
//   - HTTPS: https://github.com/owner/repo
//   - SSH colon: git@github.com:owner/repo
//   - SSH protocol: ssh://git@github.com/owner/repo
//
// The .git suffix should be removed by the caller before calling [ExtractPathComponents].
package urlutil

import (
	"net/url"
	"strings"
)

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

// Host returns the bare hostname of a git remote URL, lowercased, with any port,
// userinfo and trailing dot removed. It handles the same three formats as
// [ExtractPathComponents] plus bracketed IPv6 literals:
//
//	Host("https://gitlab.com/owner/repo.git")        → "gitlab.com"
//	Host("git@codeberg.org:owner/repo.git")          → "codeberg.org"
//	Host("ssh://git@codeberg.org:22/owner/repo.git") → "codeberg.org"
//	Host("git@[::1]:owner/repo.git")                 → "::1"
//
// It returns an empty string when no network host can be determined — an empty or
// malformed input, a local filesystem path, or a scheme such as file:// that has no
// host. Callers comparing hosts for authentication decisions must therefore treat
// the empty string as "no match" so that unparseable remotes fail closed.
//
// The comparison unit is deliberately the hostname alone, ignoring the port: a
// self-hosted instance commonly serves SSH and HTTP on different ports of the same
// host, and those are the same trust domain.
func Host(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	if host, decided := hostFromURL(trimmed); decided {
		return host
	}
	return hostFromSCP(trimmed)
}

// hostFromURL resolves a scheme-qualified URL such as https://host/path or
// ssh://user@host:port/path. The second result reports whether the input's form
// determines the answer; when false the caller should try SCP-style parsing instead.
func hostFromURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if parsed.Host != "" {
		return canonicalHost(parsed.Hostname()), true // Hostname() drops port and userinfo.
	}
	// A scheme with no host is only a genuine host-less URL (file:///path) when the
	// remainder is hierarchical. An opaque remainder means url.Parse read a
	// scheme-less SCP target such as "codeberg.org:owner/repo.git" as
	// scheme="codeberg.org", opaque="owner/repo.git" — that needs SCP parsing.
	if parsed.Scheme != "" && parsed.Opaque == "" {
		return "", true
	}
	return "", false
}

// hostFromSCP resolves the scp-like "[user@]host:path" form, including the
// bracketed IPv6 variant "user@[::1]:path". It returns an empty string for input
// with no host component, such as a bare local path.
func hostFromSCP(rawURL string) string {
	remainder := rawURL
	if at := strings.LastIndex(remainder, "@"); at != -1 {
		remainder = remainder[at+1:]
	}

	if strings.HasPrefix(remainder, "[") {
		end := strings.Index(remainder, "]")
		if end == -1 {
			return ""
		}
		return canonicalHost(remainder[1:end])
	}

	host, _, found := strings.Cut(remainder, ":")
	if !found || host == "" {
		return "" // A local path such as /a/b, or unparseable input.
	}
	return canonicalHost(host)
}

// canonicalHost normalizes a hostname for comparison: lowercased, with the
// fully-qualified trailing dot removed so "gitlab.com." matches "gitlab.com".
func canonicalHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
