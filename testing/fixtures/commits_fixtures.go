package fixtures

import (
	"time"

	"github.com/sgaunet/auto-mr/pkg/commits"
)

const (
	// TestCommitHashFull is a full SHA-1 hash for testing.
	TestCommitHashFull = "abc123def456789012345678901234567890abcd"
	// TestCommitHashShort is an abbreviated hash for testing.
	TestCommitHashShort = "abc123d"
	// TestAuthor is a test commit author.
	TestAuthor = "Test Developer <test@example.com>"
)

// SingleCommit returns a single valid commit for testing auto-selection scenarios.
func SingleCommit() []commits.Commit {
	return []commits.Commit{
		{
			Hash:         TestCommitHashFull,
			ShortHash:    TestCommitHashShort,
			Message:      "feat: add user authentication",
			Title:        "feat: add user authentication",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 10, 30, 0, 0, time.UTC),
			ParentHashes: []string{"def456abc789012345678901234567890abcdef1"},
		},
	}
}

// MultipleCommits returns multiple valid commits for testing interactive selection scenarios.
func MultipleCommits() []commits.Commit {
	return []commits.Commit{
		{
			Hash:         "ghi789012345678901234567890123456789abcd",
			ShortHash:    "ghi789c",
			Message:      "fix: handle edge case for expired tokens",
			Title:        "fix: handle edge case for expired tokens",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 12, 0, 0, 0, time.UTC),
			ParentHashes: []string{"def456abc789012345678901234567890abcdef2"},
		},
		{
			Hash:         "def456789012345678901234567890123456abcd",
			ShortHash:    "def456b",
			Message:      "feat: add JWT token validation",
			Title:        "feat: add JWT token validation",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 11, 0, 0, 0, time.UTC),
			ParentHashes: []string{"abc123def456789012345678901234567890abcd"},
		},
		{
			Hash:         TestCommitHashFull,
			ShortHash:    TestCommitHashShort,
			Message:      "WIP: start authentication work",
			Title:        "WIP: start authentication work",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 10, 0, 0, 0, time.UTC),
			ParentHashes: []string{"def456abc789012345678901234567890abcdef3"},
		},
	}
}

// CommitWithMultiLineMessage returns a commit with title and body for testing message parsing.
func CommitWithMultiLineMessage() commits.Commit {
	return commits.Commit{
		Hash:      TestCommitHashFull,
		ShortHash: TestCommitHashShort,
		Message: `feat: add dark mode

Implemented theme switching with these features:
- Toggle in settings panel
- System preference detection
- User preference persistence
- Smooth transition animations`,
		Title: "feat: add dark mode",
		Body: `Implemented theme switching with these features:
- Toggle in settings panel
- System preference detection
- User preference persistence
- Smooth transition animations`,
		Author:       TestAuthor,
		Timestamp:    time.Date(2025, 1, 11, 10, 30, 0, 0, time.UTC),
		ParentHashes: []string{"def456abc789012345678901234567890abcdef4"},
	}
}

// CommitsWithMerges returns commits including merge commits for testing filtering.
func CommitsWithMerges() []commits.Commit {
	return []commits.Commit{
		{
			Hash:         TestCommitHashFull,
			ShortHash:    TestCommitHashShort,
			Message:      "feat: add authentication",
			Title:        "feat: add authentication",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 10, 30, 0, 0, time.UTC),
			ParentHashes: []string{"parent1"},
		},
		{
			Hash:         "merge123456789012345678901234567890abcd",
			ShortHash:    "merge12",
			Message:      "Merge branch 'main' into feature",
			Title:        "Merge branch 'main' into feature",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 11, 0, 0, 0, time.UTC),
			ParentHashes: []string{"parent1", "parent2"}, // Merge commit has 2 parents
		},
		{
			Hash:         "ghi789012345678901234567890123456789abcd",
			ShortHash:    "ghi789c",
			Message:      "fix: handle edge case",
			Title:        "fix: handle edge case",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 12, 0, 0, 0, time.UTC),
			ParentHashes: []string{"parent3"},
		},
	}
}

// CommitsWithEmptyMessages returns commits with empty or whitespace-only messages for testing validation.
func CommitsWithEmptyMessages() []commits.Commit {
	return []commits.Commit{
		{
			Hash:         TestCommitHashFull,
			ShortHash:    TestCommitHashShort,
			Message:      "",
			Title:        "",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 10, 30, 0, 0, time.UTC),
			ParentHashes: []string{"parent1"},
		},
		{
			Hash:         "def456789012345678901234567890123456abcd",
			ShortHash:    "def456b",
			Message:      "   ",
			Title:        "   ",
			Body:         "",
			Author:       TestAuthor,
			Timestamp:    time.Date(2025, 1, 11, 11, 0, 0, 0, time.UTC),
			ParentHashes: []string{"parent2"},
		},
	}
}

// ValidMessageSelection returns a valid message selection for testing.
func ValidMessageSelection() commits.MessageSelection {
	return commits.MessageSelection{
		Title:            "feat: add user authentication",
		Body:             "",
		SourceCommitHash: TestCommitHashFull,
		SelectionMethod:  commits.SelectionAuto,
		ManualOverride:   false,
	}
}

// ManualMessageSelection returns a manual override message selection for testing.
func ManualMessageSelection() commits.MessageSelection {
	return commits.MessageSelection{
		Title:            "Custom MR title",
		Body:             "Custom MR description",
		SourceCommitHash: "",
		SelectionMethod:  commits.SelectionManual,
		ManualOverride:   true,
	}
}

// ValidCommitList returns a valid commit list for testing.
func ValidCommitList() commits.CommitList {
	allCommits := CommitsWithMerges()
	return commits.CommitList{
		All: allCommits,
		Valid: []commits.Commit{
			allCommits[0], // Non-merge commit
			allCommits[2], // Non-merge commit
		},
		Branch:             "feature/authentication",
		RetrievalTimestamp: time.Now(),
	}
}
