package commits

import (
	"strings"
	"time"
)

const (
	// DefaultDisplayTitleLength is the default max length for commit titles in display.
	DefaultDisplayTitleLength = 80
)

// SelectionMethod represents how the message was selected.
type SelectionMethod int

const (
	// SelectionAuto indicates single commit auto-selected (no user prompt).
	SelectionAuto SelectionMethod = iota
	// SelectionInteractive indicates user selected from multiple commits via UI.
	SelectionInteractive
	// SelectionManual indicates user provided custom message via -msg flag.
	SelectionManual
)

// Commit represents a single git commit with its metadata and message content.
type Commit struct {
	// Hash is the full SHA-1 hash of the commit (40 characters).
	Hash string
	// ShortHash is the abbreviated hash for display (first 7 characters).
	ShortHash string
	// Message is the full commit message (title + body, preserving formatting).
	Message string
	// Title is the first line of commit message (used for MR/PR title).
	Title string
	// Body is the remaining lines after first line (used for MR/PR description).
	Body string
	// Author is the commit author name and email.
	Author string
	// Timestamp is when the commit was created.
	Timestamp time.Time
	// ParentHashes contains SHA hashes of parent commits (empty for initial commit, 2+ for merge commits).
	ParentHashes []string
}

// IsValid returns true if message is non-empty after trimming whitespace.
func (c *Commit) IsValid() bool {
	return strings.TrimSpace(c.Message) != ""
}

// IsMergeCommit returns true if commit has 2+ parent commits.
func (c *Commit) IsMergeCommit() bool {
	return len(c.ParentHashes) > 1
}

// TitleTruncated returns title truncated to maxLen with "..." suffix if longer.
func (c *Commit) TitleTruncated(maxLen int) string {
	if len(c.Title) <= maxLen {
		return c.Title
	}
	return c.Title[:maxLen-3] + "..."
}

// FormattedForDisplay returns "[ShortHash] TitleTruncated(DefaultDisplayTitleLength)" for UI display.
func (c *Commit) FormattedForDisplay() string {
	return "[" + c.ShortHash + "] " + c.TitleTruncated(DefaultDisplayTitleLength)
}

// CommitList represents a collection of commits from a branch, with filtering and selection capabilities.
type CommitList struct {
	// All contains all commits retrieved from git history (including merge commits).
	All []Commit
	// Valid contains filtered list excluding merge commits and empty messages.
	Valid []Commit
	// Branch is the name of the branch these commits belong to.
	Branch string
	// RetrievalTimestamp is when the commits were retrieved.
	RetrievalTimestamp time.Time
}

// Count returns the number of valid commits.
func (cl *CommitList) Count() int {
	return len(cl.Valid)
}

// HasSingleCommit returns true if exactly 1 valid commit exists.
func (cl *CommitList) HasSingleCommit() bool {
	return len(cl.Valid) == 1
}

// HasMultipleCommits returns true if 2+ valid commits exist.
func (cl *CommitList) HasMultipleCommits() bool {
	return len(cl.Valid) > 1
}

// IsEmpty returns true if zero valid commits exist.
func (cl *CommitList) IsEmpty() bool {
	return len(cl.Valid) == 0
}

// MessageSelection represents the result of the commit message selection process.
type MessageSelection struct {
	// Title is the MR/PR title (first line of selected message).
	Title string
	// Body is the MR/PR description (remaining lines of selected message).
	Body string
	// SourceCommitHash is the hash of the commit the message came from (empty if manual override).
	SourceCommitHash string
	// SelectionMethod indicates how the message was selected (AUTO, INTERACTIVE, MANUAL).
	SelectionMethod SelectionMethod
	// ManualOverride is true if -msg flag was used.
	ManualOverride bool
}

// FullMessage returns title + "\n\n" + body (reconstructed full message).
func (ms *MessageSelection) FullMessage() string {
	if ms.Body == "" {
		return ms.Title
	}
	return ms.Title + "\n\n" + ms.Body
}

// IsFromCommit returns true if SourceCommitHash is non-empty.
func (ms *MessageSelection) IsFromCommit() bool {
	return ms.SourceCommitHash != ""
}

// IsManualOverride returns true if message was provided via -msg flag.
func (ms *MessageSelection) IsManualOverride() bool {
	return ms.ManualOverride
}
