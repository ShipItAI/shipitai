# ShipItAI - Claude Code Project Documentation

## Overview

An open-source GitHub App that automatically reviews pull requests using Claude AI, posting inline comments via GitHub's review feature. Designed for self-hosting with Docker and PostgreSQL.

## Architecture

```
GitHub Webhooks → Server → Claude API
                    ↓
              GitHub API (post review)
                    ↓
              PostgreSQL (store context)
```

## Project Structure

```
shipitai/
├── cmd/
│   ├── server/main.go            # Production HTTP server (PostgreSQL, graceful shutdown, JSON logging)
│   └── local/main.go             # Local development server (no database, debug logging, reads key from file)
├── review/
│   ├── reviewer.go               # Core review orchestration (chunking, rich context)
│   ├── chunker.go                # Diff chunking for large PRs
│   ├── chunker_test.go           # Chunker tests
│   ├── context.go                # Rich context types (FileContext, RelatedFile, etc.)
│   ├── context_fetcher.go        # Fetches full files, test files, imports, commit history
│   ├── imports.go                # Language detection and import parsing
│   ├── imports_test.go           # Import parsing tests
│   ├── reply.go                  # Reply handling for follow-up questions
│   ├── prompt.go                 # Claude prompt construction (with context support)
│   ├── prompt_test.go            # Prompt tests
│   ├── parser.go                 # Parse Claude response to comments, validate line numbers
│   └── parser_test.go            # Parser tests
├── github/
│   ├── client.go                 # GitHub API client with App auth
│   ├── graphql.go                # GraphQL queries for PR review threads
│   ├── webhook.go                # Webhook parsing & signature verification
│   ├── webhook_test.go           # Webhook tests
│   └── types.go                  # GitHub API types
├── config/
│   ├── config.go                 # Load repo config file
│   └── config_test.go            # Config tests
├── storage/
│   ├── interface.go              # Storage interface for multiple backends
│   ├── types.go                  # Shared types (Installation, ReviewContext, etc.)
│   └── postgres/                 # PostgreSQL implementation (self-hosted)
│       ├── postgres.go
│       └── json.go
├── anthropic/
│   └── validate.go               # API key validation helper
├── examples/
│   ├── docker-compose.yml        # Docker Compose for self-hosted deployment
│   ├── .env.example              # Environment variables template
│   ├── shipitai.yml              # Full configuration example
│   └── shipitai.minimal.yml      # Minimal configuration example
├── docs/
│   ├── self-hosting.md           # Self-hosting guide
│   └── github-app-setup.md       # GitHub App setup guide
├── Dockerfile                    # Docker image for self-hosted deployment
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Key Components

### GitHub Client (`github/client.go`)
- Authenticates as a GitHub App installation
- Fetches PR diffs and file metadata
- Posts reviews with inline comments
- Uses `ghinstallation` for JWT-based authentication
- Checks user permissions for contributor protection (`GetUserPermission`, `IsContributor`)
- Posts issue comments for non-contributor PR notifications (`CreateIssueComment`)

### Webhook Handler (`github/webhook.go`)
- Verifies webhook signatures using HMAC-SHA256
- Parses pull_request and pull_request_review_comment events
- Filters for actionable events (opened, synchronize, reopened)
- Extracts @shipitai mentions from review comments (`ExtractMentionContext`)

### Reviewer (`review/reviewer.go`)
- Orchestrates the full review flow
- Loads repo config, fetches diff, calls Claude, posts review
- Stores review context in database (via `storage.Storage` interface)
- Supports chunked reviews for large PRs (>100KB)
- Fetches rich context (full files, test files, imports, commit history) for better reviews
- Extensible via `APIKeyFunc` callback for custom API key resolution

### Rich Context (`review/context*.go`, `imports.go`)
- Fetches full file content for modified files (not just the diff)
- Finds and fetches related test files based on language conventions
- Parses imports to find related local files
- Fetches recent commit history for modified files
- All context is fetched on-demand and never stored (privacy by design)
- Budget-based fetching with configurable limits (100KB total default)

### Chunker (`review/chunker.go`)
- Handles large PRs by splitting diffs into file-based chunks
- Chunk threshold: 100KB (~25K tokens)
- Max chunk size: 80KB (~20K tokens)
- Processes all chunks in parallel using goroutines
- Merges chunk responses: combines comments, concatenates summaries, uses strictest approval

### Claude Integration (`review/prompt.go`, `parser.go`)
- Builds structured prompts for code review
- Builds chunked prompts with chunk context (X of Y, file list)
- Parses JSON responses into GitHub review comments
- Handles markdown code block wrapping in responses
- **Validates comment line numbers** against diff hunks before posting to GitHub (prevents 422 errors from invalid line references)

### Config Loader (`config/config.go`)
- Fetches `.github/shipitai.yml` from repositories
- Supports `enabled` (bool), `trigger` (auto/on-request), `exclude` (glob patterns), and `instructions` (custom guidance)
- Fetches `CLAUDE.md` for project context (checks root first, then `.github/CLAUDE.md`)
- Filters diffs based on exclude patterns before sending to Claude
- Falls back to defaults if config missing

### Storage Interface (`storage/interface.go`)
- `Storage` interface defines the contract for review context and installation persistence
- Methods: review CRUD (StoreReview, GetReview, ListReviewsForPR, GetFirstReviewForPR) and installation management (SaveInstallation, GetInstallation)
- PostgreSQL implementation in `storage/postgres/` for self-hosted deployments
- Shared types in `storage/types.go` (Installation, ReviewContext, TokenUsage, Comment)

### Reply Handler (`review/reply.go`)
- Handles follow-up questions via `@shipitai` comment mentions
- Loads previous review context from storage
- Builds conversation-aware prompts for Claude
- Posts reply as a new review comment

## Configuration

### Repository Config (`.github/shipitai.yml`)
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

| Option | Values | Description |
|--------|--------|-------------|
| `enabled` | `true`/`false` | Enable or disable reviews for this repo |
| `trigger` | `auto` / `on-request` | When to trigger reviews |
| `exclude` | list of patterns | Glob patterns for files to skip |
| `instructions` | text | Custom guidance for the reviewer |
| `context` | object | Configure rich context fetching (see below) |
| `contributor_protection` | `true`/`false` | Restrict auto-reviews to contributors only (default: `true`) |

### Contributor Protection

Contributor protection prevents token-burning attacks by restricting automatic reviews to repository contributors only. When a non-contributor opens a PR, ShipItAI posts a message explaining that a contributor can trigger a review by commenting `@shipitai review`.

**Note:** This protection is automatically skipped for private repositories since only users with repository access can open PRs.

**How it works:**
1. Private repos: Protection skipped (only authorized users can open PRs)
2. Public repos: When a non-contributor opens a PR, ShipItAI posts an informational comment (no review)
3. A contributor can trigger a review by commenting `@shipitai review`
4. Once approved, subsequent pushes to the PR will auto-review (approval persisted 30 days)
5. On API errors, the system fails open (proceeds with review) to avoid blocking legitimate PRs

**To disable contributor protection:**
```yaml
# .github/shipitai.yml
contributor_protection: false
```

### Rich Context Configuration

Rich context is enabled by default. You can customize or disable it:

```yaml
# .github/shipitai.yml
context:
  enabled: true       # Set to false to disable all context fetching
  full_files: true    # Include full file content (not just diff)
  related_files: true # Include test files and local imports
  history: true       # Include recent commit history
```

| Option | Default | Description |
|--------|---------|-------------|
| `context.enabled` | `true` | Master switch for all context fetching |
| `context.full_files` | `true` | Fetch complete content of modified files |
| `context.related_files` | `true` | Fetch test files and imported local files |
| `context.history` | `true` | Fetch recent commit history per file |

**Privacy Note:** All context is fetched on-demand and passed directly to Claude. It is never stored in the database.

### Project Context (`CLAUDE.md`)
ShipItAI automatically fetches `CLAUDE.md` from the repository to provide project-specific context for reviews. This is the same format used by Claude Code.

**Lookup order:**
1. `CLAUDE.md` (repository root)
2. `.github/CLAUDE.md` (fallback)

The contents are included in the system prompt under "Project Context" and help Claude understand:
- Project architecture and conventions
- Tech stack and dependencies
- Coding standards and patterns
- Areas to focus on or ignore

This is optional - if no `CLAUDE.md` exists, reviews proceed without project context.

### Environment Variables (Self-Hosted)

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_APP_ID` | Yes | Your GitHub App ID |
| `GITHUB_WEBHOOK_SECRET` | Yes | Webhook signature verification secret |
| `GITHUB_PRIVATE_KEY` | Yes | GitHub App private key (PEM format) |
| `ANTHROPIC_API_KEY` | Yes | Anthropic API key for Claude |
| `DATABASE_URL` | Yes | PostgreSQL connection string (auto-configured in Docker Compose) |
| `BOT_NAME` | No | Bot username for @mentions (default: shipitai) |
| `PORT` | No | HTTP server port (default: 8080) |

## Build & Run

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Run linter
make lint

# Build Docker image
make docker-build

# Run Docker container
make docker-run

# Local development (reads from env vars)
make dev
```

## Development Notes

- Uses Claude Sonnet 4 for code reviews
- Packages are top-level (not `internal/`) to support extensibility and external imports
- `storage.Storage` interface allows plugging in different backends
- `review.APIKeyFunc` callback allows custom API key resolution without coupling to specific implementations

### Large PR Handling (Chunked Reviews)
Large PRs (>100KB diff) are automatically split into chunks and reviewed in parallel:
1. After filtering, check if `len(diff) > 100KB`
2. Split diff on `diff --git` boundaries into file diffs
3. Greedily pack files into chunks until 80KB reached
4. Process all chunks in parallel using goroutines with errgroup
5. Merge responses: concatenate comments, combine summaries, strictest approval wins
6. Post single GitHub review with merged results

**Approval merge logic:** `request_changes` > `comment` > `approve`

### Rich Context for Reviews
Reviews automatically include rich context beyond just the diff:

**What's included:**
1. **Full file content** - Complete source files being modified (50% of budget)
2. **Related test files** - Corresponding test files (25% of budget)
3. **Local imports** - Files imported by the modified code (15% of budget)
4. **Commit history** - 5 most recent commits per modified file (10% of budget)

**Size limits:**
- Per file: 50KB max (truncated with notice if exceeded)
- Total context: 100KB budget
- For chunked reviews: budget is divided among chunks

**Language-specific test file detection:**
| Language | Test Pattern |
|----------|-------------|
| Go | `foo.go` → `foo_test.go` |
| TypeScript/JS | `foo.ts` → `foo.test.ts`, `foo.spec.ts`, `__tests__/foo.ts` |
| Python | `foo.py` → `test_foo.py`, `tests/test_foo.py` |
| Ruby | `foo.rb` → `foo_spec.rb` |
| Java/Kotlin | `Foo.java` → `FooTest.java` (in test directory) |

**Import detection:**
- Go: Imports matching the module path (from go.mod)
- TypeScript/JS: Relative imports (`./`, `../`) and `@/` style imports
- Python: Relative imports (`from .module import ...`)
