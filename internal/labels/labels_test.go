package labels_test

import (
	"testing"

	"github.com/sgaunet/auto-mr/internal/labels"
)

func TestExtractCommitType(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"type with scope", "feat(ui): add button", "feat"},
		{"type without scope", "fix: resolve crash", "fix"},
		{"docs type", "docs: update README", "docs"},
		{"refactor type", "refactor(api): simplify handler", "refactor"},
		{"test type", "test: add unit tests", "test"},
		{"ci type", "ci: update pipeline", "ci"},
		{"style type", "style: format code", "style"},
		{"perf type", "perf(db): optimize query", "perf"},
		{"build type", "build: update deps", "build"},
		{"chore type", "chore: cleanup", "chore"},
		{"revert type", "revert: undo change", "revert"},
		{"empty string", "", ""},
		{"no colon", "just a message", ""},
		{"colon at start", ": no type", ""},
		{"uppercase type", "FEAT: something", ""},
		{"mixed case", "Feat: something", ""},
		{"special chars in type", "feat!: breaking", ""},
		{"space before colon", "feat : message", "feat"},
		{"nested parens", "feat(scope)(extra): msg", "feat"},
		{"empty scope", "feat(): msg", "feat"},
		{"type with numbers", "v2: something", "v2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labels.ExtractCommitType(tt.title)
			if got != tt.want {
				t.Errorf("ExtractCommitType(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestAutoSelectLabels(t *testing.T) {
	availableLabels := []string{"bug", "Feature", "enhancement", "documentation", "CI/CD", "chore"}

	tests := []struct {
		name      string
		title     string
		available []string
		want      []string
	}{
		{
			name:      "feat matches feature and enhancement",
			title:     "feat: add login",
			available: availableLabels,
			want:      []string{"Feature", "enhancement"},
		},
		{
			name:      "fix matches bug",
			title:     "fix(auth): resolve crash",
			available: availableLabels,
			want:      []string{"bug"},
		},
		{
			name:      "docs matches documentation",
			title:     "docs: update README",
			available: availableLabels,
			want:      []string{"documentation"},
		},
		{
			name:      "ci matches CI/CD case-insensitive",
			title:     "ci: update pipeline",
			available: availableLabels,
			want:      []string{"CI/CD"},
		},
		{
			name:      "chore matches chore",
			title:     "chore: cleanup",
			available: availableLabels,
			want:      []string{"chore"},
		},
		{
			name:      "no match returns nil",
			title:     "feat: add feature",
			available: []string{"random", "other"},
			want:      nil,
		},
		{
			name:      "non-conventional commit returns nil",
			title:     "just a message",
			available: availableLabels,
			want:      nil,
		},
		{
			name:      "empty title returns nil",
			title:     "",
			available: availableLabels,
			want:      nil,
		},
		{
			name:      "empty available labels returns nil",
			title:     "feat: add feature",
			available: []string{},
			want:      nil,
		},
		{
			name:      "unknown type returns nil",
			title:     "custom: something",
			available: availableLabels,
			want:      nil,
		},
		{
			name:      "nil available labels returns nil",
			title:     "feat: add feature",
			available: nil,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labels.AutoSelectLabels(tt.title, tt.available)
			if !stringSliceEqual(got, tt.want) {
				t.Errorf("AutoSelectLabels(%q, %v) = %v, want %v", tt.title, tt.available, got, tt.want)
			}
		})
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		// Treat nil and empty as equal only if both are nil or both are empty
		return (a == nil) == (b == nil)
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
