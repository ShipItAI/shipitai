# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please report security vulnerabilities by emailing:

**security@shipitai.dev**

Please include:

1. **Description** of the vulnerability
2. **Steps to reproduce** the issue
3. **Potential impact** of the vulnerability
4. **Suggested fix** (if any)

### What to Expect

- **Acknowledgment**: We will acknowledge receipt of your report within 48 hours
- **Assessment**: We will assess the vulnerability and determine its severity
- **Updates**: We will keep you informed of our progress
- **Resolution**: We aim to resolve critical vulnerabilities within 7 days
- **Credit**: We will credit you in the security advisory (unless you prefer to remain anonymous)

### Scope

The following are in scope:
- The ShipItAI application code
- Authentication and authorization mechanisms
- Data handling and storage
- API endpoints
- Webhook signature verification

The following are out of scope:
- Third-party dependencies (report to the respective maintainers)
- Social engineering attacks
- Denial of service attacks

## Security Best Practices for Self-Hosted Deployments

If you're self-hosting ShipItAI, please ensure:

1. **Webhook Secret**: Use a strong, random webhook secret (at least 32 characters)
2. **API Keys**: Keep your Anthropic API key secure; never commit it to version control
3. **GitHub Private Key**: Store the GitHub App private key securely
4. **Network Security**: Use HTTPS for all webhook endpoints
5. **Database**: Use strong passwords and restrict database access
6. **Updates**: Keep your deployment updated with the latest releases

## Security Features

ShipItAI includes several security features:

- **Webhook Signature Verification**: All GitHub webhooks are verified using HMAC-SHA256
- **Contributor Protection**: Optional protection against malicious PRs from non-contributors
- **No Code Storage**: Source code is processed in memory and not stored

## Acknowledgments

We thank the security researchers who have helped improve ShipItAI's security.
