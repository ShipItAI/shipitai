# GitHub App Setup Guide

This guide walks through creating a GitHub App for the code reviewer.

## Prerequisites

- A GitHub account
- (Optional) A GitHub organization for testing

## Step 1: Create the GitHub App

1. Go to your GitHub Settings:
   - For a personal app: `https://github.com/settings/apps`
   - For an organization app: `https://github.com/organizations/<YOUR_ORG>/settings/apps`

2. Click **"New GitHub App"**

3. Fill in the basic information:
   - **GitHub App name**: Choose a unique name (e.g., `your-org-shipitai`)
   - **Description**: "AI-powered code reviewer for pull requests"
   - **Homepage URL**: Your app's URL (can use a placeholder like `https://example.com` for now)

## Step 2: Configure URLs

- **Webhook URL**: `https://<your-domain>/webhooks/github`
  - For local dev with ngrok: `https://<ngrok-subdomain>.ngrok.io/webhooks/github`
  - For local dev with smee: Use your smee.io channel URL

- **Webhook secret**: Generate a strong secret and save it securely
  ```bash
  openssl rand -hex 32
  ```

## Step 3: Set Permissions

Under **"Permissions & events"**, configure the following:

### Repository Permissions

| Permission | Access Level | Reason |
|------------|--------------|--------|
| **Contents** | Read | Read repository files and diffs |
| **Pull requests** | Read & Write | Read PR details, post reviews and comments |
| **Metadata** | Read | Required for all GitHub Apps |

### Organization Permissions

None required.

### Account Permissions

None required.

## Step 4: Subscribe to Events

Under **"Subscribe to events"**, enable:

- [x] **Pull request** - Triggered when PRs are opened, updated, closed
- [x] **Pull request review comment** - Triggered when someone replies to review comments (for @mention replies)

## Step 5: Installation Options

- **Where can this GitHub App be installed?**
  - Select **"Any account"** if you want others to install it (SaaS model)
  - Select **"Only on this account"** for private/internal use

## Step 6: Create the App

Click **"Create GitHub App"**

## Step 7: Generate a Private Key

After creation, you'll be on the app's settings page:

1. Scroll down to **"Private keys"**
2. Click **"Generate a private key"**
3. A `.pem` file will be downloaded - **store this securely**
4. This key is used to authenticate as the GitHub App

## Step 8: Note Your App Credentials

Save these values (you'll need them for configuration):

- **App ID**: Shown at the top of the app settings page
- **Private key**: The `.pem` file you downloaded
- **Webhook secret**: The secret you generated in Step 2

## Step 9: Install the App

1. Go to `https://github.com/apps/<your-app-name>`
2. Click **"Install"** or **"Configure"**
3. Choose the account/organization
4. Select repositories:
   - **All repositories**, or
   - **Only select repositories** (recommended for testing)
5. Click **"Install"**

## Local Development Setup

### Option A: Using ngrok

1. Install ngrok: `brew install ngrok` (macOS) or download from ngrok.com
2. Start ngrok: `ngrok http 8080`
3. Copy the `https://` URL (e.g., `https://abc123.ngrok.io`)
4. Update your GitHub App's webhook URL to: `https://abc123.ngrok.io/webhooks/github`

### Option B: Using smee.io

1. Go to https://smee.io and click "Start a new channel"
2. Copy the channel URL
3. Install smee client: `npm install -g smee-client`
4. Run: `smee -u https://smee.io/YOUR_CHANNEL -t http://localhost:8080/webhooks/github`
5. Set your GitHub App's webhook URL to the smee.io channel URL

## Environment Variables

Your application will need these environment variables:

```bash
# GitHub App credentials
GITHUB_APP_ID=123456
GITHUB_PRIVATE_KEY_PATH=/path/to/private-key.pem
GITHUB_WEBHOOK_SECRET=your-webhook-secret
ANTHROPIC_API_KEY=sk-ant-api03-your-key-here
```

## Verifying the Setup

1. Start your local server on port 8080
2. Start ngrok or smee
3. Create a test PR in an installed repository
4. Check your server logs for the incoming webhook
5. Verify the webhook signature matches

## Security Notes

- **Never commit** the private key or webhook secret to version control
- Use a secrets manager for production deployments
- Rotate the webhook secret periodically
- The private key should have restricted file permissions (`chmod 600`)

## Troubleshooting

### Webhook not received
- Verify the webhook URL is correct and accessible
- Check GitHub App settings > Advanced > Recent Deliveries for errors
- Ensure ngrok/smee is running

### Authentication errors
- Verify the App ID is correct
- Check the private key file is valid and readable
- Ensure the app is installed on the repository

### Permission errors
- Verify the app has the required permissions
- Re-install the app if permissions were changed after installation
