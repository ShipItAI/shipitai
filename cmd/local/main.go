// Package main provides a local development server for testing webhooks.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/shipitai/shipitai/github"
	"github.com/shipitai/shipitai/review"
)

var (
	logger         *slog.Logger
	webhookHandler *github.WebhookHandler
	reviewer       *review.Reviewer
	githubClient   *github.Client
	botName        string
)

func main() {
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	if err := initialize(); err != nil {
		logger.Error("failed to initialize", "error", err)
		os.Exit(1)
	}

	http.HandleFunc("/webhooks/github", handleWebhook)
	http.HandleFunc("/health", handleHealth)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("starting local server", "port", port)
	logger.Info("webhook endpoint", "url", fmt.Sprintf("http://localhost:%s/webhooks/github", port))

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func initialize() error {
	// Load config from environment variables
	webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if webhookSecret == "" {
		return fmt.Errorf("GITHUB_WEBHOOK_SECRET is required")
	}

	privateKeyPath := os.Getenv("GITHUB_PRIVATE_KEY_PATH")
	if privateKeyPath == "" {
		return fmt.Errorf("GITHUB_PRIVATE_KEY_PATH is required")
	}

	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key from %s: %w", privateKeyPath, err)
	}

	claudeAPIKey := os.Getenv("ANTHROPIC_API_KEY")
	if claudeAPIKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is required")
	}

	appIDStr := os.Getenv("GITHUB_APP_ID")
	if appIDStr == "" {
		return fmt.Errorf("GITHUB_APP_ID is required")
	}

	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
	}

	// Bot name for mention detection (defaults to "shipitai")
	botName = os.Getenv("BOT_NAME")
	if botName == "" {
		botName = "shipitai"
	}

	// Initialize components
	webhookHandler = github.NewWebhookHandler(webhookSecret)
	githubClient = github.NewClient(appID, privateKey)

	// No database in local mode
	reviewer = review.NewReviewer(githubClient, claudeAPIKey, nil, logger)

	// Optional: override the default Claude model
	if model := os.Getenv("ANTHROPIC_MODEL"); model != "" {
		reviewer.SetModel(model)
	}

	logger.Info("initialized", "app_id", appID, "bot_name", botName)
	return nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("failed to read body", "error", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Get event type
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	logger.Info("received webhook", "event", eventType, "size", len(payload))

	// Verify signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if err := webhookHandler.VerifySignature(payload, signature); err != nil {
		logger.Error("signature verification failed", "error", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Handle ping
	if eventType == "ping" {
		logger.Info("received ping")
		jsonResponse(w, http.StatusOK, map[string]string{"message": "pong"})
		return
	}

	// Handle review comment events (for @mentions)
	if eventType == "pull_request_review_comment" {
		handleReviewComment(w, payload)
		return
	}

	// Only handle pull_request events
	if eventType != "pull_request" {
		logger.Info("ignoring event", "type", eventType)
		jsonResponse(w, http.StatusOK, map[string]string{"message": "event ignored"})
		return
	}

	// Parse event
	event, err := webhookHandler.ParsePullRequestEvent(payload)
	if err != nil {
		logger.Error("failed to parse event", "error", err)
		http.Error(w, "failed to parse event", http.StatusBadRequest)
		return
	}

	// Check if we should process
	if !webhookHandler.ShouldProcess(eventType, event) {
		logger.Info("skipping event", "action", event.Action)
		jsonResponse(w, http.StatusOK, map[string]string{"message": "event skipped"})
		return
	}

	logger.Info("processing PR",
		"repo", event.Repository.FullName,
		"pr", event.Number,
		"action", event.Action,
	)

	// Respond immediately to GitHub, then process async
	// (Claude API can take longer than GitHub's 10s timeout)
	jsonResponse(w, http.StatusOK, map[string]string{"message": "review started"})

	// Perform review in background with detached context
	input := &review.ReviewInput{
		InstallationID: event.Installation.ID,
		Owner:          event.Repository.Owner.Login,
		Repo:           event.Repository.Name,
		PRNumber:       event.Number,
		PRTitle:        event.PullRequest.Title,
		PRBody:         event.PullRequest.Body,
		HeadSHA:        event.PullRequest.Head.SHA,
		DefaultBranch:  event.Repository.DefaultBranch,
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		result, err := reviewer.Review(ctx, input)
		if err != nil {
			logger.Error("review failed", "error", err)
			return
		}

		if result == nil {
			logger.Info("review skipped (not enabled)")
			return
		}

		logger.Info("review posted",
			"review_id", result.ReviewID,
			"comments", result.CommentCount,
			"url", result.ReviewURL,
		)
	}()
}

func handleReviewComment(w http.ResponseWriter, payload []byte) {
	event, err := webhookHandler.ParseReviewCommentEvent(payload)
	if err != nil {
		logger.Error("failed to parse review comment event", "error", err)
		http.Error(w, "failed to parse event", http.StatusBadRequest)
		return
	}

	// Check if we should process this comment
	if !webhookHandler.ShouldProcessComment(event, botName) {
		logger.Info("ignoring comment (no mention or not created)",
			"action", event.Action,
			"body_preview", truncate(event.Comment.Body, 50),
		)
		jsonResponse(w, http.StatusOK, map[string]string{"message": "comment ignored"})
		return
	}

	logger.Info("processing @mention",
		"repo", event.Repository.FullName,
		"pr", event.PullRequest.Number,
		"comment_id", event.Comment.ID,
		"user", event.Sender.Login,
	)

	// Respond immediately
	jsonResponse(w, http.StatusOK, map[string]string{"message": "reply started"})

	// Process in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Fetch all comments to build thread context
		comments, err := githubClient.GetReviewComments(
			ctx,
			event.Installation.ID,
			event.Repository.Owner.Login,
			event.Repository.Name,
			event.PullRequest.Number,
		)
		if err != nil {
			logger.Error("failed to fetch comments", "error", err)
			return
		}

		threadContext := review.BuildThreadContext(comments, event.Comment.ID)
		userQuestion := github.ExtractMentionContext(event.Comment.Body, botName)

		input := &review.ReplyInput{
			InstallationID: event.Installation.ID,
			Owner:          event.Repository.Owner.Login,
			Repo:           event.Repository.Name,
			PRNumber:       event.PullRequest.Number,
			CommentID:      event.Comment.ID,
			DiffHunk:       event.Comment.DiffHunk,
			FilePath:       event.Comment.Path,
			UserQuestion:   userQuestion,
			ThreadContext:  threadContext,
		}

		result, err := reviewer.Reply(ctx, input)
		if err != nil {
			logger.Error("reply failed", "error", err)
			return
		}

		logger.Info("reply posted",
			"comment_id", result.CommentID,
			"url", result.CommentURL,
		)
	}()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
