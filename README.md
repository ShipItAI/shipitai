# ShipItAI

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shipitai/shipitai)](go.mod)

AI-powered code reviews for GitHub pull requests, powered by Claude.

## Features

- **Automatic PR Reviews** - Reviews triggered on PR open, synchronize, and reopen events
- **Inline Comments** - Precise feedback on specific lines of code
- **Rich Context** - Full file content, related tests, import analysis, and commit history
- **Large PR Support** - Intelligent chunking for PRs over 100KB
- **Configurable** - Per-repository settings via `.github/shipitai.yml`
- **Follow-up Replies** - Reply to review comments with `@shipitai` for clarification
- **Contributor Protection** - Prevents token-burning from untrusted PRs on public repos
- **Self-Hosted** - Deploy on your own infrastructure with Docker and PostgreSQL

## Quick Start

See the [Self-Hosting Guide](docs/self-hosting.md) for Docker-based deployment, or the [GitHub App Setup Guide](docs/github-app-setup.md) for creating the GitHub App.

## Configuration

Create `.github/shipitai.yml` in your repository:

```yaml
enabled: true
trigger: auto  # "auto" | "on-request"

# Skip generated code, vendored dependencies, etc.
exclude:
  - vendor/**
  - "*.gen.go"
  - docs/**

# Custom guidance for the reviewer
instructions: |
  Focus on security issues.
  We use sqlc for database queries.
```

### Options

| Option | Values | Description |
|--------|--------|-------------|
| `enabled` | `true`/`false` | Enable or disable reviews |
| `trigger` | `auto` / `on-request` | When to trigger reviews |
| `exclude` | list of patterns | Glob patterns for files to skip |
| `instructions` | text | Custom guidance for the reviewer |
| `context.enabled` | `true`/`false` | Enable rich context fetching |
| `contributor_protection` | `true`/`false` | Restrict auto-reviews to contributors |

See [examples/shipitai.yml](examples/shipitai.yml) for a full configuration example.

### Project Context

ShipItAI automatically reads `CLAUDE.md` from your repository root (or `.github/CLAUDE.md`) to understand your project's architecture, conventions, and coding standards.

## How It Works

1. GitHub sends webhook when PR is opened/synchronized
2. Fetches `.github/shipitai.yml` and `CLAUDE.md` from the repository
3. Fetches PR diff and rich context (full files, tests, imports, history)
4. Sends to Claude with review instructions
5. Parses response into inline comments
6. Posts GitHub review with feedback

## Review Philosophy

The reviewer focuses on:
- Bugs and logic errors
- Security vulnerabilities
- Performance issues
- Significant code clarity problems

It does NOT comment on:
- Minor style preferences
- Formatting (assumes automated formatters)
- Self-explanatory code

## Architecture

```
GitHub Webhooks → Server → Claude API
                    ↓
              GitHub API (post review)
                    ↓
              PostgreSQL (store context)
```

## Development

### Prerequisites

- Go 1.23+
- PostgreSQL (for self-hosted server)
- GitHub App configured ([setup guide](docs/github-app-setup.md))

### Setup

```bash
# Install dependencies
make deps

# Run tests
make test

# Run linter
make lint

# Build Docker image
make docker-build
```

### Entry Points

The project has two server commands:

| Command | Purpose | Database | Use When |
|---------|---------|----------|----------|
| `cmd/server` | Production self-hosted server | PostgreSQL (required) | Deploying with Docker |
| `cmd/local` | Local development server | None | Testing webhooks locally |

The local server skips database storage and reads the GitHub private key from a file path (`GITHUB_PRIVATE_KEY_PATH`) instead of an environment variable, making it easier to iterate during development.

### Local Development

```bash
# Start local dev server (no database required)
make dev

# Use ngrok to expose webhook endpoint
ngrok http 8080
```

### Dependencies

This project uses the [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go) for Claude API integration. The SDK is currently in alpha (`v0.2.0-alpha.6`) as the official Go SDK tracks toward a stable release.

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Security

Report security vulnerabilities via the process described in [SECURITY.md](SECURITY.md).

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
