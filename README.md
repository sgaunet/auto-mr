# auto-mr

A Go-based automated merge request tool for GitLab, GitHub, and self-hosted Forgejo repositories. This tool eliminates the need for external CLI dependencies by using native Go libraries.

## Features

- ✅ Zero external CLI dependencies (replaces `glab`, `gh`, `jq`, `yq`, `gum`)
- ✅ Support for GitLab, GitHub, and self-hosted Forgejo
- ✅ Interactive label selection
- ✅ Pipeline/workflow waiting with timeout
- ✅ Auto-approval and merging
- ✅ Branch cleanup after merge
- ✅ Configuration via YAML file

## Installation

### From Releases

* Download the latest release from the [releases page](https://github.com/sgaunet/auto-mr/releases).
* Install the binary in /usr/local/bin or any other directory in your PATH.

### With go

```bash
go install github.com/sgaunet/auto-mr@latest
```

### From source:

```bash
git clone https://github.com/sgaunet/auto-mr
cd auto-mr
go build -o auto-mr
```

### Homebrew

```bash
brew tap sgaunet/homebrew-tools
brew install sgaunet/tools/auto-mr
```

## Configuration

Create a configuration file at `~/.config/auto-mr/config.yml`:

```yaml
gitlab:
  assignee: your-gitlab-username
  reviewer: reviewer-gitlab-username
github:
  assignee: your-github-username
  reviewer: reviewer-github-username
forgejo:
  url: https://forgejo.example.com   # required: your self-hosted Forgejo instance URL
  assignee: your-forgejo-username
  reviewer: reviewer-forgejo-username
```

The `forgejo` section is only required when working with a Forgejo repository. The instance URL (`url`) is mandatory because Forgejo is self-hosted — there is no default host. Platform detection matches the git remote host against the configured `forgejo.url`.

## Environment Variables

### GitLab
Set your GitLab personal access token:
```bash
export GITLAB_TOKEN="your-gitlab-token"
```

### GitHub
Set your GitHub personal access token:
```bash
export GITHUB_TOKEN="your-github-token"
```

### Forgejo
Set your Forgejo personal access token:
```bash
export FORGEJO_TOKEN="your-forgejo-token"
```

## Usage

1. Make sure you're on a feature branch (not main/master)
2. Commit and ensure there are no staged changes
3. Run the tool:

```bash
auto-mr
```

### Options

- `--squash`: Squash commits when merging (default: false, preserves commit history)
- `--log-level`: Set log level (debug, info, warn, error) (default: "info")
- `--version`: Print version and exit

Example with squash:
```bash
auto-mr --squash
```

### Workflow

The tool will:
1. Detect if you're using GitLab, GitHub, or Forgejo
2. Push your current branch
3. Let you select labels interactively
4. Create a merge/pull request with proper assignee and reviewer
5. Wait for CI/CD pipeline completion
6. Auto-approve (GitLab only; Forgejo and GitHub skip this step) and merge the request (squash if --squash flag is used)
7. Switch back to main branch and clean up

## Replaced Dependencies

This Go version eliminates these external dependencies:

| Original Tool | Replaced With |
|---------------|---------------|
| `glab` | GitLab Go client library |
| `gh` | GitHub Go client library |
| `jq` | Native Go JSON processing |
| `yq` | Native Go YAML processing |
| `gum` | Survey library for interactive prompts |
| `git` | go-git library |

## Token Permissions

### GitLab Token Permissions
- `api` (full API access)
- `read_repository`
- `write_repository`

### GitHub Token Permissions
- `repo` (full repository access)
- `workflow` (if using GitHub Actions)

### Forgejo Token Permissions
- `repository` (read and write access to repositories)
- `issue` (read and write access — required to manage pull requests)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test ./...`
5. Submit a pull request

## License

MIT License - see LICENSE file for details.