// Package labels provides automatic label selection based on conventional commit types.
package labels

import "strings"

// commitTypeToLabels maps conventional commit types to candidate label names.
var commitTypeToLabels = map[string][]string{
	"feat":     {"feature", "enhancement"},
	"fix":      {"bug", "bugfix", "fix"},
	"docs":     {"documentation", "docs"},
	"refactor": {"refactor", "refactoring", "tech-debt"},
	"test":     {"test", "testing", "tests"},
	"ci":       {"ci", "ci/cd", "infrastructure"},
	"style":    {"style", "formatting"},
	"perf":     {"performance", "perf", "optimization"},
	"build":    {"build", "dependencies"},
	"chore":    {"chore", "maintenance"},
	"revert":   {"revert"},
}

// ExtractCommitType parses the conventional commit type from a title.
// Supports "type(scope): msg" and "type: msg" formats.
// Returns "" if the title doesn't follow conventional commit format.
func ExtractCommitType(title string) string {
	if title == "" {
		return ""
	}

	// Find the colon separator
	colonIdx := strings.Index(title, ":")
	if colonIdx < 1 {
		return ""
	}

	prefix := title[:colonIdx]

	// Strip optional scope: "feat(ui)" → "feat"
	if parenIdx := strings.Index(prefix, "("); parenIdx > 0 {
		prefix = prefix[:parenIdx]
	}

	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}

	// Validate: type must be lowercase alphanumeric
	for _, c := range prefix {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return ""
		}
	}

	return prefix
}

// AutoSelectLabels selects labels automatically based on the commit title's
// conventional commit type. It returns the original label names from
// availableLabels that match the commit type's candidates (case-insensitive).
// Returns nil if no matches are found.
func AutoSelectLabels(title string, availableLabels []string) []string {
	commitType := ExtractCommitType(title)
	if commitType == "" {
		return nil
	}

	candidates, ok := commitTypeToLabels[commitType]
	if !ok {
		return nil
	}

	// Build a lowercase→original map for available labels
	availableMap := make(map[string]string, len(availableLabels))
	for _, label := range availableLabels {
		availableMap[strings.ToLower(label)] = label
	}

	var matched []string
	for _, candidate := range candidates {
		if original, found := availableMap[strings.ToLower(candidate)]; found {
			matched = append(matched, original)
		}
	}

	return matched
}
