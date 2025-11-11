package commits_test

import (
	"testing"

	"github.com/sgaunet/auto-mr/pkg/commits"
	"github.com/sgaunet/auto-mr/testing/fixtures"
)

// T010 [P] [US1] Test Commit.IsValid() method
func TestCommit_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		commit   commits.Commit
		expected bool
	}{
		{
			name:     "valid commit with message",
			commit:   fixtures.SingleCommit()[0],
			expected: true,
		},
		{
			name:     "invalid commit with empty message",
			commit:   fixtures.CommitsWithEmptyMessages()[0],
			expected: false,
		},
		{
			name:     "invalid commit with whitespace-only message",
			commit:   fixtures.CommitsWithEmptyMessages()[1],
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.commit.IsValid()
			if got != tt.expected {
				t.Errorf("IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// T011 [P] [US1] Test Commit.IsMergeCommit() method
func TestCommit_IsMergeCommit(t *testing.T) {
	tests := []struct {
		name     string
		commit   commits.Commit
		expected bool
	}{
		{
			name:     "regular commit with single parent",
			commit:   fixtures.SingleCommit()[0],
			expected: false,
		},
		{
			name:     "merge commit with two parents",
			commit:   fixtures.CommitsWithMerges()[1],
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.commit.IsMergeCommit()
			if got != tt.expected {
				t.Errorf("IsMergeCommit() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// T012 [P] [US1] Test Commit.TitleTruncated() method
func TestCommit_TitleTruncated(t *testing.T) {
	const shortMaxLen = 50
	const longMaxLen = 20

	tests := []struct {
		name        string
		commit      commits.Commit
		maxLen      int
		expectTrunc bool
	}{
		{
			name:        "short title not truncated",
			commit:      fixtures.SingleCommit()[0],
			maxLen:      shortMaxLen,
			expectTrunc: false,
		},
		{
			name: "long title truncated with ellipsis",
			commit: commits.Commit{
				Title: "This is a very long commit title that exceeds the maximum length",
			},
			maxLen:      longMaxLen,
			expectTrunc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.commit.TitleTruncated(tt.maxLen)
			if tt.expectTrunc {
				if len(got) != tt.maxLen {
					t.Errorf("TitleTruncated(%d) length = %d, want %d", tt.maxLen, len(got), tt.maxLen)
				}
				if got[len(got)-3:] != "..." {
					t.Errorf("TitleTruncated(%d) = %q, expected to end with '...'", tt.maxLen, got)
				}
			} else {
				if got != tt.commit.Title {
					t.Errorf("TitleTruncated(%d) = %q, want %q", tt.maxLen, got, tt.commit.Title)
				}
			}
		})
	}
}

// T013 [P] [US1] Test Commit.FormattedForDisplay() method
func TestCommit_FormattedForDisplay(t *testing.T) {
	const displayMaxLen = 80

	commit := fixtures.SingleCommit()[0]
	got := commit.FormattedForDisplay()

	// Should contain short hash
	if !contains(got, commit.ShortHash) {
		t.Errorf("FormattedForDisplay() = %q, expected to contain short hash %q", got, commit.ShortHash)
	}

	// Should be formatted as "[hash] title"
	expectedPrefix := "[" + commit.ShortHash + "] "
	if !startsWithStr(got, expectedPrefix) {
		t.Errorf("FormattedForDisplay() = %q, expected to start with %q", got, expectedPrefix)
	}

	// Total length should not exceed hash + brackets + space + 80 chars
	maxExpectedLen := len(expectedPrefix) + displayMaxLen
	if len(got) > maxExpectedLen {
		t.Errorf("FormattedForDisplay() length = %d, should not exceed %d", len(got), maxExpectedLen)
	}
}

// T014 [P] [US1] Test CommitList.Count() method
func TestCommitList_Count(t *testing.T) {
	const expectedValidCount = 2

	commitList := fixtures.ValidCommitList()
	got := commitList.Count()

	if got != expectedValidCount {
		t.Errorf("Count() = %d, want %d", got, expectedValidCount)
	}
}

// T015 [P] [US1] Test CommitList.HasSingleCommit() and HasMultipleCommits() methods
func TestCommitList_HasSingleCommit(t *testing.T) {
	tests := []struct {
		name             string
		commitList       commits.CommitList
		expectSingle     bool
		expectMultiple   bool
	}{
		{
			name: "single commit",
			commitList: commits.CommitList{
				Valid: fixtures.SingleCommit(),
			},
			expectSingle:   true,
			expectMultiple: false,
		},
		{
			name:           "multiple commits",
			commitList:     fixtures.ValidCommitList(),
			expectSingle:   false,
			expectMultiple: true,
		},
		{
			name: "no commits",
			commitList: commits.CommitList{
				Valid: []commits.Commit{},
			},
			expectSingle:   false,
			expectMultiple: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSingle := tt.commitList.HasSingleCommit()
			gotMultiple := tt.commitList.HasMultipleCommits()

			if gotSingle != tt.expectSingle {
				t.Errorf("HasSingleCommit() = %v, want %v", gotSingle, tt.expectSingle)
			}
			if gotMultiple != tt.expectMultiple {
				t.Errorf("HasMultipleCommits() = %v, want %v", gotMultiple, tt.expectMultiple)
			}
		})
	}
}

// T016 [P] [US1] Test CommitList.IsEmpty() method
func TestCommitList_IsEmpty(t *testing.T) {
	tests := []struct {
		name       string
		commitList commits.CommitList
		expected   bool
	}{
		{
			name: "empty list",
			commitList: commits.CommitList{
				Valid: []commits.Commit{},
			},
			expected: true,
		},
		{
			name:       "non-empty list",
			commitList: fixtures.ValidCommitList(),
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.commitList.IsEmpty()
			if got != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Helper functions
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func startsWithStr(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
