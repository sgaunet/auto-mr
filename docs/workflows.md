# Development Workflows

## Feature Development

1. Create a feature branch from `main`
2. Implement changes with black box tests (`package foo_test`)
3. Run `task lint` and `task test` locally
4. Open a PR/MR — CI runs `task lint` and `task test` automatically
5. Merge after CI passes (no formal review requirement configured; `CONTRIBUTING.md` does not exist)

## Code Review Process

- Automated checks (`.github/workflows/linter.yml`, `test.yml`) must pass before merge
- No `CONTRIBUTING.md` present — add one if review requirements need to be codified

## Testing Strategy

- Unit tests are colocated with source (`pkg/*/`, `internal/*/`) as `*_test.go`, using external `_test` packages
- Run with `task test` → `go test -count=2 -race ./...` (race detector, double run to catch flakiness)
- Coverage: `task coverage` generates a filtered `go tool cover` report (excludes `cmd/` paths)
- `internal/logger/` is held to 100% coverage; see [patterns.md](patterns.md) for conventions

## Release Process

- Automated via GitHub Actions:
  - `.github/workflows/snapshot.yml` — snapshot builds via goreleaser on every push to `main`
  - `.github/workflows/release.yml` — full release via goreleaser, triggered by pushing a `v*` tag
- Tool versions (go, task, golangci-lint, goreleaser, syft) are pinned in `mise.toml` and installed via `jdx/mise-action` in CI
