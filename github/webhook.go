package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidSignature indicates the webhook signature verification failed.
	ErrInvalidSignature = errors.New("invalid webhook signature")
	// ErrMissingSignature indicates the webhook signature header is missing.
	ErrMissingSignature = errors.New("missing webhook signature")
	// ErrUnsupportedEvent indicates the webhook event type is not handled.
	ErrUnsupportedEvent = errors.New("unsupported event type")
)

// WebhookHandler handles GitHub webhook events.
type WebhookHandler struct {
	secret []byte
}

// NewWebhookHandler creates a new webhook handler with the given secret.
func NewWebhookHandler(secret string) *WebhookHandler {
	return &WebhookHandler{
		secret: []byte(secret),
	}
}

// VerifySignature verifies the webhook payload signature.
// The signature header should be in the format "sha256=<hex-encoded-signature>".
func (h *WebhookHandler) VerifySignature(payload []byte, signatureHeader string) error {
	if signatureHeader == "" {
		return ErrMissingSignature
	}

	// Parse signature header (format: sha256=<signature>)
	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return ErrInvalidSignature
	}

	signature, err := hex.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Compute expected signature
	mac := hmac.New(sha256.New, h.secret)
	mac.Write(payload)
	expected := mac.Sum(nil)

	// Compare signatures using constant-time comparison
	if !hmac.Equal(signature, expected) {
		return ErrInvalidSignature
	}

	return nil
}

// ParsePullRequestEvent parses a pull_request webhook payload.
func (h *WebhookHandler) ParsePullRequestEvent(payload []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	if event.PullRequest == nil {
		return nil, errors.New("payload is not a pull request event")
	}

	return &event, nil
}

// ShouldProcess determines if the event should trigger a review.
// Returns true for pull_request events with actions: opened, synchronize, reopened.
func (h *WebhookHandler) ShouldProcess(eventType string, event *WebhookEvent) bool {
	if eventType != "pull_request" {
		return false
	}

	switch event.Action {
	case "opened", "synchronize", "reopened":
		return true
	default:
		return false
	}
}

// ParseReviewCommentEvent parses a pull_request_review_comment webhook payload.
func (h *WebhookHandler) ParseReviewCommentEvent(payload []byte) (*ReviewCommentEvent, error) {
	var event ReviewCommentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse review comment payload: %w", err)
	}

	if event.Comment == nil {
		return nil, errors.New("payload is missing comment")
	}

	return &event, nil
}

// ShouldProcessComment determines if a review comment should trigger a reply.
// Returns true if the comment mentions the bot name and action is "created".
func (h *WebhookHandler) ShouldProcessComment(event *ReviewCommentEvent, botName string) bool {
	if event.Action != "created" {
		return false
	}

	if event.Comment == nil {
		return false
	}

	return ContainsMention(event.Comment.Body, botName)
}

// ContainsMention checks if text contains an @mention of the given username.
// It ensures the mention is a proper GitHub-style mention (not part of an email address).
func ContainsMention(text, username string) bool {
	lowerText := strings.ToLower(text)
	mention := "@" + strings.ToLower(username)

	idx := 0
	for {
		pos := strings.Index(lowerText[idx:], mention)
		if pos == -1 {
			return false
		}
		pos += idx // Adjust to absolute position

		// Check character before: must be start of string or non-alphanumeric
		if pos > 0 {
			before := lowerText[pos-1]
			if isAlphanumeric(before) {
				idx = pos + 1
				continue
			}
		}

		// Check character after: must be end of string, non-alphanumeric,
		// or not a domain pattern (dot followed by letter)
		afterPos := pos + len(mention)
		if afterPos < len(lowerText) {
			after := lowerText[afterPos]
			// If followed by alphanumeric, not a valid mention
			if isAlphanumeric(after) {
				idx = pos + 1
				continue
			}
			// If followed by dot, check if it's a domain (dot + letter)
			if after == '.' && afterPos+1 < len(lowerText) && isLetter(lowerText[afterPos+1]) {
				idx = pos + 1
				continue
			}
		}

		return true
	}
}

func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ExtractMentionContext extracts the user's question/request after removing the @mention.
func ExtractMentionContext(text, username string) string {
	mention := "@" + username
	// Case-insensitive replacement
	lower := strings.ToLower(text)
	mentionLower := strings.ToLower(mention)

	idx := strings.Index(lower, mentionLower)
	if idx == -1 {
		return strings.TrimSpace(text)
	}

	// Remove the mention and clean up
	result := text[:idx] + text[idx+len(mention):]
	return strings.TrimSpace(result)
}

// ParseInstallationEvent parses an installation webhook payload.
func (h *WebhookHandler) ParseInstallationEvent(payload []byte) (*InstallationEvent, error) {
	var event InstallationEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse installation payload: %w", err)
	}

	if event.Installation == nil {
		return nil, errors.New("payload is missing installation")
	}

	return &event, nil
}

// ParseIssueCommentEvent parses an issue_comment webhook payload.
func (h *WebhookHandler) ParseIssueCommentEvent(payload []byte) (*IssueCommentEvent, error) {
	var event IssueCommentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse issue comment payload: %w", err)
	}

	if event.Comment == nil {
		return nil, errors.New("payload is missing comment")
	}

	if event.Issue == nil {
		return nil, errors.New("payload is missing issue")
	}

	return &event, nil
}

// ShouldProcessIssueComment determines if an issue comment should trigger a review.
// Returns true if:
// - The action is "created"
// - The issue is a pull request (has a pull_request link)
// - The comment mentions the bot name
func (h *WebhookHandler) ShouldProcessIssueComment(event *IssueCommentEvent, botName string) bool {
	if event.Action != "created" {
		return false
	}

	if event.Issue == nil || event.Issue.PullRequest == nil {
		return false // Not a PR comment
	}

	if event.Comment == nil {
		return false
	}

	return ContainsMention(event.Comment.Body, botName)
}

// ExtractCommand extracts a command from a comment body after an @mention.
// Returns the command (e.g., "review") or empty string if no valid command found.
// Example: "@shipitai review" -> "review"
// Example: "@shipitai please review this" -> "review"
func ExtractCommand(text, botName string) string {
	lowerText := strings.ToLower(text)
	mention := "@" + strings.ToLower(botName)

	idx := strings.Index(lowerText, mention)
	if idx == -1 {
		return ""
	}

	// Get the text after the mention
	afterMention := strings.TrimSpace(lowerText[idx+len(mention):])

	// Look for "review" command anywhere in the remaining text
	if strings.Contains(afterMention, "review") {
		return "review"
	}

	return ""
}
