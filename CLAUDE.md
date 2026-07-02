# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Operating Guidelines

**Read `docs/operating-guidelines.md` at the start of every session.** It
defines how to plan, verify, and iterate in this repository: plan mode,
subagent strategy, verification gates, self-improvement loop, and the
communication contract. Treat it as load-bearing context.

## Project Overview

`auto-mr` is a Go CLI tool that automates merge/pull request workflows for GitLab, GitHub, and Forgejo (self-hosted). It uses native Go libraries for git operations, API interactions, and interactive prompts — no external CLI dependencies required (except `git` for cleanup operations). Licensed under MIT.

## Build and Development Commands

Tool versions (go, task, golangci-lint, goreleaser, syft) are pinned in `mise.toml`.

```bash
task build              # Build binary (CGO_ENABLED=0)
task lint               # Run golangci-lint
task test               # Run tests with race detector (go test -count=2 -race ./...)
task coverage           # Generate coverage report
task snapshot           # Snapshot release via goreleaser
go run . --log-level debug  # Run with debug logging
```

## Architecture

Pipeline pattern in `main.go`: validate branch → detect platform → push & create MR/PR → wait for CI → merge → cleanup.

| Package | Purpose |
|---|---|
| `pkg/git/` | Git operations (go-git for push/auth, native git for cleanup) |
| `pkg/gitlab/` | GitLab API client with job-level CI visualization |
| `pkg/github/` | GitHub API client with check-level workflow visualization |
| `pkg/forgejo/` | Forgejo (Gitea) API client via `code.gitea.io/sdk/gitea`, commit-status CI visualization |
| `pkg/config/` | Config from `~/.config/auto-mr/config.yml` |
| `pkg/commits/` | Commit message parsing and generation |
| `pkg/platform/` | Platform adapter abstraction |
| `internal/logger/` | Structured logging via `log/slog` |
| `internal/ui/` | Interactive terminal prompts (survey/v2) |
| `internal/security/` | Token sanitization and secure error wrapping |
| `internal/timeutil/` | Duration formatting utilities |
| `testing/mocks/` | Mock implementations for black box testing |
| `testing/fixtures/` | Factory functions for realistic test data |

See [docs/architecture.md](docs/architecture.md) for detailed design decisions and patterns.

## Code Quality

**Linters configured** (do not duplicate rules):
- **golangci-lint v2**: See `.golangci.yml` — all linters enabled, 12 explicitly disabled
- **pre-commit**: See `.pre-commit-config.yaml` — runs test/lint/build hooks before each commit
- **goreleaser**: See `.goreleaser.yml` for release config

## Environment Variables

- `GITLAB_TOKEN` — GitLab personal access token (required for GitLab repos)
- `GITHUB_TOKEN` — GitHub personal access token (required for GitHub repos)
- `FORGEJO_TOKEN` — Forgejo personal access token (required for Forgejo repos)

## Configuration

Config file: `~/.config/auto-mr/config.yml` — requires `assignee` and `reviewer` per platform (gitlab/github/forgejo). Optional `pipeline_timeout` per platform (default: 30m, range: 1m–8h). CLI flag `--pipeline-timeout` takes highest priority.

Forgejo requires an additional `url` field (the self-hosted instance base URL, e.g. `https://forgejo.example.com`). Platform detection matches the git remote host against the configured `forgejo.url`.

## Testing

Black box testing with external test packages (`_test` suffix) and mock implementations:
- `pkg/github/` — client, workflows, errors, edge cases
- `pkg/gitlab/` — client, workflows, errors, edge cases
- `pkg/forgejo/` — client construction, commit-status aggregation, error sentinels
- `pkg/config/`, `pkg/git/`, `pkg/platform/` — config validation, platform detection, adapter behavior
- `internal/logger/` — 100% coverage

See [docs/patterns.md](docs/patterns.md) for testing methodology and conventions.

## Documentation

- [docs/architecture.md](docs/architecture.md): System design and component overview
- [docs/workflows.md](docs/workflows.md): Development processes and git workflow
- [docs/patterns.md](docs/patterns.md): Code patterns and best practices
- [docs/operating-guidelines.md](docs/operating-guidelines.md): Session workflow, verification, and self-improvement rules

## Version

Injected at build time via ldflags: `-X main.version={{.Version}}`. Access via `--version` flag.
