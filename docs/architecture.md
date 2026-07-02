# Architecture

## Overview

`auto-mr` follows a pipeline pattern orchestrated in `main.go`:

```
validate branch → detect platform → push → create MR/PR → wait for CI → merge → cleanup
```

Platform detection is driven by matching the git remote URL against the configured platform URLs and known hosted domains (github.com, gitlab.com, or the custom `forgejo.url`).

## Package Structure

| Package | Purpose |
|---|---|
| `pkg/git/` | Git operations (go-git for push/auth, native git for cleanup) |
| `pkg/gitlab/` | GitLab API client — job-level CI visualization via pipeline/job polling |
| `pkg/github/` | GitHub API client — check-run workflow visualization |
| `pkg/forgejo/` | Forgejo (Gitea) API client — commit-status CI visualization via `GetCombinedStatus` |
| `pkg/config/` | Config from `~/.config/auto-mr/config.yml`; validates all platform sections |
| `pkg/commits/` | Conventional commit message parsing and MR/PR title generation |
| `pkg/platform/` | Platform adapter abstraction (`Provider` interface + per-platform adapters) |
| `internal/logger/` | Structured logging via `log/slog` |
| `internal/ui/` | Interactive terminal prompts (survey/v2) |
| `internal/security/` | Token sanitization and secure error wrapping |
| `internal/timeutil/` | Human-readable duration formatting |
| `testing/mocks/` | Mock implementations for black-box tests |
| `testing/fixtures/` | Factory functions for realistic test data |

## Platform Abstraction (`pkg/platform/`)

All three platforms share a single `Provider` interface:

```go
type Provider interface {
    Initialize(remoteURL string) error
    ListLabels() ([]Label, error)
    Create(params CreateParams) (*MergeRequest, error)
    GetByBranch(sourceBranch, targetBranch string) (*MergeRequest, error)
    WaitForPipeline(timeout time.Duration) (string, error)
    Approve(mrID int64) error
    Merge(params MergeParams) error
    PlatformName() string
    PipelineTimeout() string
}
```

Concrete adapters (`GitLabAdapter`, `GitHubAdapter`, `ForgejoAdapter`) are created by `factory.go` and implement this interface. The pipeline in `main.go` operates exclusively through the `Provider` interface.

### Adapter Behaviours

| Behaviour | GitLab | GitHub | Forgejo |
|---|---|---|---|
| CI source | Pipeline/job polling | Check runs | Commit statuses (`GetCombinedStatus`) |
| Approve | API call (required for merge) | No-op | No-op |
| Branch deletion | Via merge API option | Via merge API | Via `DeleteBranchAfterMerge` in merge option |

### Forgejo Adapter (`ForgejoAdapter`)

`ForgejoAdapter` wraps `*forgejo.Client` and translates between the platform-agnostic `Provider` interface and the Forgejo-specific API from `code.gitea.io/sdk/gitea`.

Key design decisions:

- **No approval step**: Forgejo does not gate merges on formal approvals; `Approve` is a deliberate no-op.
- **CI via commit statuses**: `WaitForPipeline` calls `GetCombinedStatus` in a poll loop (5 s interval). Individual status contexts are visualised with animated spinners. Repos with no statuses configured are treated as "no CI" after a brief grace period.
- **Automatic branch cleanup**: `MergePullRequest` always passes `DeleteBranchAfterMerge: true` to the Gitea SDK, so the source branch is removed by the server on merge.
- **Platform detection**: `main.go` compares the git remote host against the `forgejo.url` value from config. Because Forgejo is self-hosted there is no default host.

## Configuration

Config file: `~/.config/auto-mr/config.yml`

```yaml
gitlab:
  assignee: alice
  reviewer: bob
  pipeline_timeout: 45m   # optional, default 30m
github:
  assignee: alice
  reviewer: bob
github:
  assignee: alice
  reviewer: bob
forgejo:
  url: https://forgejo.example.com   # required for Forgejo; no default
  assignee: alice
  reviewer: bob
  pipeline_timeout: 60m   # optional
```

Environment variables supply tokens; they are never stored in the config file:

| Variable | Platform |
|---|---|
| `GITLAB_TOKEN` | GitLab |
| `GITHUB_TOKEN` | GitHub |
| `FORGEJO_TOKEN` | Forgejo |

## Testing Strategy

All tests use black-box style (`package foo_test`). Live-server tests are skipped when the remote host is unreachable. See `docs/patterns.md` for detailed testing conventions.

### Forgejo test coverage

- `pkg/forgejo/forgejo_test.go` — pure-logic tests: error sentinels, SDK constant values, `NewClient` token-required guard, `Label` type, error wrapping/unwrapping.
- `pkg/platform/provider_test.go` — Forgejo adapter interface tests via mock: platform name, no-op approve, merge params with source branch.
