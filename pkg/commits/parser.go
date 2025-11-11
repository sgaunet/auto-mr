package commits

import (
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// ParseCommit converts a go-git Commit to our domain Commit type.
func ParseCommit(gitCommit *object.Commit) Commit {
	title, body := ParseCommitMessage(gitCommit.Message)

	parentHashes := make([]string, len(gitCommit.ParentHashes))
	for i, hash := range gitCommit.ParentHashes {
		parentHashes[i] = hash.String()
	}

	return Commit{
		Hash:         gitCommit.Hash.String(),
		ShortHash:    gitCommit.Hash.String()[:7],
		Message:      gitCommit.Message,
		Title:        title,
		Body:         body,
		Author:       gitCommit.Author.Name + " <" + gitCommit.Author.Email + ">",
		Timestamp:    gitCommit.Author.When,
		ParentHashes: parentHashes,
	}
}

// ParseCommitMessage splits commit message into title (first line) and body (remaining lines).
// Title and body are trimmed of whitespace.
// Returns empty body if commit message is single-line.
func ParseCommitMessage(fullMessage string) (string, string) {
	lines := strings.Split(fullMessage, "\n")
	title := strings.TrimSpace(lines[0])
	body := ""

	if len(lines) > 1 {
		// Join remaining lines, preserve formatting
		bodyLines := lines[1:]
		body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	}

	return title, body
}

// FilterValidCommits returns commits that are not merge commits and have non-empty messages.
func FilterValidCommits(commits []Commit) []Commit {
	valid := make([]Commit, 0, len(commits))

	for _, c := range commits {
		if !c.IsMergeCommit() && c.IsValid() {
			valid = append(valid, c)
		}
	}

	return valid
}

// BuildCommitList constructs a CommitList with filtering applied.
func BuildCommitList(all []Commit, branch string) CommitList {
	return CommitList{
		All:                all,
		Valid:              FilterValidCommits(all),
		Branch:             branch,
		RetrievalTimestamp: time.Now(),
	}
}
