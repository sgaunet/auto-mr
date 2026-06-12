package forgejo

import (
	"fmt"
	"net/http"
	"strings"

	"code.gitea.io/sdk/gitea"
	"github.com/sgaunet/auto-mr/internal/urlutil"
)

// SetRepositoryFromURL sets the repository from a git remote URL.
// Supports both HTTPS and SSH URL formats:
//   - https://forgejo.example.com/owner/repo.git
//   - git@forgejo.example.com:owner/repo.git
//
// Returns [ErrInvalidURLFormat] if the URL cannot be parsed into owner/repo.
// Returns a wrapped error if the repository does not exist or the API call fails.
func (c *Client) SetRepositoryFromURL(url string) error {
	url = strings.TrimSuffix(url, ".git")

	ownerRepo := urlutil.ExtractPathComponents(url, minURLParts)
	if ownerRepo == "" {
		return errInvalidURLFormat
	}

	parts := strings.Split(ownerRepo, "/")
	if len(parts) != minURLParts {
		return errInvalidURLFormat
	}

	c.owner = parts[0]
	c.repo = parts[1]

	c.log.Debug(fmt.Sprintf("Setting Forgejo repository: %s/%s", c.owner, c.repo))

	// Validate repository exists.
	_, _, err := c.client.GetRepo(c.owner, c.repo)
	if err != nil {
		return fmt.Errorf("failed to get repository information: %w", err)
	}

	c.log.Debug("Forgejo repository set successfully")
	return nil
}

// ListLabels returns all labels for the repository.
// [Client.SetRepositoryFromURL] must be called before this method.
//
// Returns an empty slice if no labels are configured.
func (c *Client) ListLabels() ([]Label, error) {
	c.log.Debug("Listing Forgejo labels")

	giteaLabels, _, err := c.client.ListRepoLabels(c.owner, c.repo, gitea.ListLabelsOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	result := make([]Label, len(giteaLabels))
	for i, l := range giteaLabels {
		result[i] = Label{Name: l.Name}
	}

	c.log.Debug(fmt.Sprintf("Labels retrieved, count: %d", len(result)))
	return result, nil
}

// CreatePullRequest creates a new pull request with assignee, reviewer, and labels.
// Label names are resolved to IDs via the repository's label list; names with no
// match are silently skipped.
//
// Parameters:
//   - head: the source branch name
//   - base: the target branch (e.g., "main")
//   - title: PR title (must not be empty)
//   - body: PR description
//   - assignee: Forgejo username to assign (empty string is skipped)
//   - reviewer: Forgejo username to request review from (empty or same as assignee is skipped)
//   - labels: label names to apply (may be nil)
//
// Returns [ErrPRAlreadyExists] if a PR already exists for the same branches.
// Stores the PR index and head SHA internally for use by [Client.WaitForPipeline].
func (c *Client) CreatePullRequest(
	head, base, title, body, assignee, reviewer string,
	labels []string,
) (*gitea.PullRequest, error) {
	c.log.Debug(fmt.Sprintf("Creating pull request from %s to %s", head, base))

	labelIDs, err := c.resolveLabelIDs(labels)
	if err != nil {
		return nil, err
	}

	opt := gitea.CreatePullRequestOption{
		Head:   head,
		Base:   base,
		Title:  title,
		Body:   body,
		Labels: labelIDs,
	}

	if assignee != "" {
		opt.Assignees = []string{assignee}
	}

	if reviewer != "" && reviewer != assignee {
		opt.Reviewers = []string{reviewer}
	}

	pr, resp, err := c.client.CreatePullRequest(c.owner, c.repo, opt)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusConflict {
			return nil, fmt.Errorf("%w: head=%s, base=%s: %w", errPRAlreadyExists, head, base, err)
		}

		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "already exists") {
			return nil, fmt.Errorf("%w: head=%s, base=%s: %w", errPRAlreadyExists, head, base, err)
		}

		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	c.prIndex = pr.Index
	c.prSHA = pr.Head.Sha
	c.log.Debug(fmt.Sprintf("Pull request created — index: %d, URL: %s", c.prIndex, pr.HTMLURL))
	return pr, nil
}

// GetPullRequestByBranch fetches an existing open pull request by head and base branches.
// Only the first matching PR is returned. Stores the PR index and SHA internally.
//
// Returns [ErrPRNotFound] if no open PR matches the given branches.
func (c *Client) GetPullRequestByBranch(head, base string) (*gitea.PullRequest, error) {
	prs, _, err := c.client.ListRepoPullRequests(c.owner, c.repo, gitea.ListPullRequestsOptions{
		State: gitea.StateOpen,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	for _, pr := range prs {
		if pr == nil {
			continue
		}

		if pr.Head != nil && pr.Base != nil &&
			pr.Head.Ref == head && pr.Base.Ref == base {
			c.prIndex = pr.Index
			c.prSHA = pr.Head.Sha
			return pr, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", errPRNotFound, head)
}

// MergePullRequest merges a pull request, automatically deleting the head branch.
//
// Parameters:
//   - index: the pull request index (number)
//   - squash: if true, uses squash merge; otherwise standard merge
//   - commitTitle: used as the merge commit message
func (c *Client) MergePullRequest(index int64, squash bool, commitTitle string) error {
	c.log.Debug(fmt.Sprintf("Merging pull request #%d (squash=%v)", index, squash))

	style := gitea.MergeStyleMerge
	if squash {
		style = gitea.MergeStyleSquash
	}

	d:=true
	_, _, err := c.client.MergePullRequest(c.owner, c.repo, index, gitea.MergePullRequestOption{
		Style:                  style,
		Title:                  commitTitle,
		DeleteBranchAfterMerge: &d,
	})
	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	c.log.Debug("Pull request merged successfully")
	return nil
}

// resolveLabelIDs resolves label names to their integer IDs.
// Names with no match in the repository's label list are silently skipped.
func (c *Client) resolveLabelIDs(names []string) ([]int64, error) {
	if len(names) == 0 {
		return nil, nil
	}

	repoLabels, _, err := c.client.ListRepoLabels(c.owner, c.repo, gitea.ListLabelsOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list labels for resolution: %w", err)
	}

	// Build name→ID index.
	nameToID := make(map[string]int64, len(repoLabels))
	for _, l := range repoLabels {
		nameToID[l.Name] = l.ID
	}

	ids := make([]int64, 0, len(names))
	for _, name := range names {
		if id, found := nameToID[name]; found {
			ids = append(ids, id)
		} else {
			c.log.Debug(fmt.Sprintf("Label %q not found in repository, skipping", name))
		}
	}

	return ids, nil
}
