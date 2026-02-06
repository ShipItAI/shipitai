// Package main provides a standalone HTTP server for self-hosted ShipItAI deployments.
//
// Configuration via environment variables:
//
//	GITHUB_APP_ID        - GitHub App ID (required)
//	GITHUB_WEBHOOK_SECRET - Webhook signature verification secret (required)
//	GITHUB_PRIVATE_KEY   - GitHub App private key in PEM format (required)
//	ANTHROPIC_API_KEY    - Anthropic API key for Claude (required)
//	DATABASE_URL         - PostgreSQL connection string (required)
//	PORT                 - HTTP server port (default: 8080)
//	BOT_NAME             - Bot username for @mentions (default: shipitai)
//
// Usage:
//
//	go run cmd/server/main.go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/shipitai/shipitai/github"
	"github.com/shipitai/shipitai/review"
	"github.com/shipitai/shipitai/storage"
	"github.com/shipitai/shipitai/storage/postgres"
)

var (
	logger         *slog.Logger
	webhookHandler *github.WebhookHandler
	reviewer       *review.Reviewer
	githubClient   *github.Client
	pgStorage      *postgres.PostgreSQL
	botName        string
)

func main() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := initialize(); err != nil {
		logger.Error("failed to initialize", "error", err)
		os.Exit(1)
	}
	defer pgStorage.Close()

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/webhooks/github", handleWebhook)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/", handleRoot)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 600 * time.Second, // Long timeout for Claude API calls
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("starting server", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
}

func initialize() error {
	// Load required config from environment
	webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if webhookSecret == "" {
		return fmt.Errorf("GITHUB_WEBHOOK_SECRET is required")
	}

	privateKey := os.Getenv("GITHUB_PRIVATE_KEY")
	if privateKey == "" {
		return fmt.Errorf("GITHUB_PRIVATE_KEY is required")
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

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	// Bot name for @mentions
	botName = os.Getenv("BOT_NAME")
	if botName == "" {
		botName = "shipitai"
	}

	// Initialize PostgreSQL storage
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	pgStorage = postgres.New(db)

	// Run migrations
	if err := pgStorage.Migrate(context.Background()); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize GitHub components
	webhookHandler = github.NewWebhookHandler(webhookSecret)
	githubClient = github.NewClient(appID, []byte(privateKey))

	// Initialize reviewer with PostgreSQL storage
	reviewer = review.NewReviewer(githubClient, claudeAPIKey, pgStorage, logger)

	logger.Info("initialized",
		"app_id", appID,
		"bot_name", botName,
	)

	return nil
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{
		"name":    "ShipItAI",
		"status":  "running",
		"version": "self-hosted",
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{"status": "healthy"})
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

	// Respond immediately to GitHub
	jsonResponse(w, http.StatusOK, map[string]string{"message": "review started"})

	// Perform review in background
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

	// Create or update installation record
	ctx := context.Background()
	install, _ := pgStorage.GetInstallation(ctx, event.Installation.ID)
	if install == nil {
		// Auto-create installation for self-hosted (always active)
		install = &storage.Installation{
			InstallationID: event.Installation.ID,
			OrgLogin:       event.Repository.Owner.Login,
			InstalledAt:    time.Now().UTC().Format(time.RFC3339),
		}
		if err := pgStorage.SaveInstallation(ctx, install); err != nil {
			logger.Error("failed to save installation", "error", err)
		}
	}

	go func() {
		reviewCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		result, err := reviewer.Review(reviewCtx, input)
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
		logger.Info("ignoring comment",
			"action", event.Action,
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

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
