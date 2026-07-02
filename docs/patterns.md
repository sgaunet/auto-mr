# Code Patterns & Best Practices

## Error Handling

Sentinel errors (`errXxx`, sometimes re-exported as `Err*`) defined via `errors.New`, wrapped with context using `fmt.Errorf("%w: ...", sentinel, detail)` so callers can use `errors.Is` while still getting a readable message.

```go
// pkg/platform/errors.go:5-10
var errUnsupportedPlatform = errors.New("unsupported platform")
// pkg/platform/factory.go:56
return nil, fmt.Errorf("%w: %s", errUnsupportedPlatform, remoteURL)
```

This repeats consistently across `pkg/github/`, `pkg/gitlab/`, `pkg/forgejo/`, `pkg/config/`, `pkg/platform/`, and `pkg/commits/`.

## Testing Patterns

- Test file naming: `*_test.go`, package `<name>_test` (black box only — never reach into unexported internals)
- Test organization: colocated with source; shared fixtures/mocks live in top-level `testing/fixtures/` and `testing/mocks/`
- Mocking strategy: narrow interfaces (`APIClient`, `StateTracker`, `DisplayRenderer`) satisfied by both real clients and `testing/mocks` fakes; compile-time checks via `var _ Interface = (*Type)(nil)`

## Platform Adapter Pattern

`pkg/platform/` defines a single `Provider` interface (`Initialize`, `ListLabels`, `Create`, `GetByBranch`, `WaitForPipeline`, `Approve`, `Merge`, `PlatformName`, `PipelineTimeout`). Each platform (`GitHubAdapter`, `GitLabAdapter`, `ForgejoAdapter`) wraps its concrete API client and translates platform-specific types/errors into platform-agnostic ones. `factory.go` selects the adapter at runtime from the detected git remote.

## Security: Token Sanitization

`internal/security/` centralizes credential redaction before logs or errors: regex patterns for GitLab (`glpat-`), GitHub (`ghp_`/`gho_`/`ghs_`), Authorization headers, and bearer tokens are compiled once via `sync.Once`. `SanitizeString`/`SanitizeError`/`SanitizeMap`/`MaskSSHKeyPath` are the public helpers; `SanitizeError` re-wraps under a generic sentinel so the original message never leaks.

## Module Layout

- `internal/` — unexported implementation helpers (logging, UI, security, timeutil)
- `pkg/` — importable API surface (git, github, gitlab, forgejo, config, commits, platform)
- `testing/` — shared fixtures and mocks reused by both `internal/` and `pkg/` tests without import cycles
