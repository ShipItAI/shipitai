# Self-Hosting ShipItAI

This guide covers deploying ShipItAI on your own infrastructure using Docker.

## Prerequisites

- Docker and Docker Compose
- A GitHub account (for creating a GitHub App)
- An Anthropic API key
- A public URL for webhook delivery (ngrok, Cloudflare Tunnel, or your own domain)

## Quick Start

### 1. Create a GitHub App

1. Go to [GitHub Developer Settings](https://github.com/settings/apps)
2. Click "New GitHub App"
3. Fill in the details:
   - **Name**: Your app name (e.g., "MyOrg Code Reviewer")
   - **Homepage URL**: Your website or GitHub profile
   - **Webhook URL**: `https://your-server.com/webhooks/github` (update after deployment)
   - **Webhook secret**: Generate a secure random string
4. Set permissions:
   - **Repository permissions**:
     - Contents: Read
     - Pull requests: Read and write
     - Metadata: Read
   - **Subscribe to events**:
     - Pull request
     - Pull request review comment
5. Click "Create GitHub App"
6. Generate and download a private key
7. Note your App ID

### 2. Get an Anthropic API Key

1. Go to [Anthropic Console](https://console.anthropic.com/)
2. Create an API key
3. Save it securely

### 3. Configure Environment

```bash
# Clone the repository
git clone https://github.com/shipitai/shipitai.git
cd shipitai/examples

# Copy the example environment file
cp .env.example .env

# Edit .env with your values
# - GITHUB_APP_ID
# - GITHUB_WEBHOOK_SECRET
# - GITHUB_PRIVATE_KEY (paste entire PEM content)
# - ANTHROPIC_API_KEY
```

### 4. Start the Services

```bash
docker compose up -d
```

The server will start on port 8080.

### 5. Expose the Webhook URL

For development, use ngrok:
```bash
ngrok http 8080
```

For production, use a reverse proxy (nginx, Caddy) with HTTPS.

### 6. Update GitHub App Webhook URL

Go back to your GitHub App settings and update the Webhook URL to your public URL:
```
https://your-domain.com/webhooks/github
```

### 7. Install the App

1. Go to your GitHub App's public page
2. Click "Install"
3. Select the repositories to enable

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_APP_ID` | Yes | Your GitHub App ID |
| `GITHUB_WEBHOOK_SECRET` | Yes | Webhook signature verification secret |
| `GITHUB_PRIVATE_KEY` | Yes | GitHub App private key (PEM format) |
| `ANTHROPIC_API_KEY` | Yes | Anthropic API key for Claude |
| `DATABASE_URL` | Yes | PostgreSQL connection string (auto-configured in Docker Compose) |
| `BOT_NAME` | No | Bot username for @mentions (default: shipitai) |
| `PORT` | No | HTTP server port (default: 8080) |

### Database

The Docker Compose setup includes PostgreSQL. For external databases:

```bash
DATABASE_URL=postgres://user:password@host:5432/dbname?sslmode=require
```

## Architecture

```
                    ┌─────────────┐
                    │   GitHub    │
                    │  Webhooks   │
                    └──────┬──────┘
                           │
                           ▼
┌──────────────────────────────────────────┐
│              ShipItAI Server             │
│  ┌─────────────────────────────────────┐ │
│  │         Webhook Handler             │ │
│  │  - Signature verification           │ │
│  │  - Event routing                    │ │
│  └────────────────┬────────────────────┘ │
│                   │                      │
│  ┌────────────────▼────────────────────┐ │
│  │           Reviewer                  │ │
│  │  - Config loading                   │ │
│  │  - Context fetching                 │ │
│  │  - Claude API calls                 │ │
│  │  - Response parsing                 │ │
│  └────────────────┬────────────────────┘ │
│                   │                      │
│  ┌────────────────▼────────────────────┐ │
│  │         GitHub Client               │ │
│  │  - Post reviews                     │ │
│  │  - Reply to comments                │ │
│  └─────────────────────────────────────┘ │
└──────────────────────────────────────────┘
                    │
                    ▼
            ┌───────────────┐
            │  PostgreSQL   │
            │   Database    │
            └───────────────┘
```

## Updating

To update to the latest version:

```bash
cd shipitai/examples
git pull
docker compose build
docker compose up -d
```

## Troubleshooting

### Webhook not received

1. Check the webhook URL is publicly accessible
2. Verify the webhook secret matches
3. Check GitHub App webhook delivery logs

### Reviews not posting

1. Check the app has correct permissions
2. Verify the app is installed on the repository
3. Check server logs: `docker compose logs shipitai`

### Database connection issues

1. Ensure PostgreSQL is running: `docker compose ps`
2. Check database logs: `docker compose logs postgres`

## Production Considerations

For production deployments:

1. **HTTPS**: Always use HTTPS for webhook endpoints
2. **Secrets**: Use a secrets manager for sensitive values
3. **Monitoring**: Set up logging and alerting
4. **Backups**: Configure database backups
5. **Resources**: Adjust container resources based on load
6. **Updates**: Keep the deployment updated

## Support

- Open a GitHub issue for bugs or questions
- See [SECURITY.md](../SECURITY.md) for reporting security issues
