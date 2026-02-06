package review

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/shipitai/shipitai/config"
	"github.com/shipitai/shipitai/github"
	"github.com/shipitai/shipitai/storage"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// APIKeyFunc is a function that resolves the API key for a given installation.
// It returns the API key, whether it's a custom (per-installation) key, and any error.
// If the function returns an error or is nil, the default API key is used.
type APIKeyFunc func(ctx context.Context, installationID int64) (apiKey string, isCustomKey bool, err error)

// ModelFunc is a function that resolves the Claude model for a given installation.
// It returns the model ID to use. If it returns an empty string or error, the default model is used.
type ModelFunc func(ctx context.Context, installationID int64) (string, error)

// ModelOption describes a supported Claude model for selection.
type ModelOption struct {
	ID    string
	Label string
}

// SupportedModels lists the Claude models available for per-installation selection.
// The first entry is the default selection.
var SupportedModels = []ModelOption{
	{ID: "claude-sonnet-4-5-20250929", Label: "Claude Sonnet 4.5"},
	{ID: "claude-opus-4-6", Label: "Claude Opus 4.6"},
	{ID: "claude-haiku-4-5-20251001", Label: "Claude Haiku 4.5"},
}

// IsValidModel checks whether a model ID is in the supported models list.
func IsValidModel(id string) bool {
	for _, m := range SupportedModels {
		if m.ID == id {
			return true
		}
	}
	return false
}

const (
	// DefaultModel is the Claude model used for code reviews.
	DefaultModel = "claude-sonnet-4-20250514"

	// ClaudeAPITimeout is the maximum time to wait for a Claude API response.
	ClaudeAPITimeout = 3 * time.Minute

	// MaxConcurrentChunks limits how many chunks can be reviewed in parallel.
	MaxConcurrentChunks = 5

	// MaxRetries is the number of times to retry transient API failures.
	MaxRetries = 3

	// RetryBaseDelay is the initial delay between retries (doubles each attempt).
	RetryBaseDelay = 1 * time.Second
)

// isRetryableError checks if an error is transient and worth retrying.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Retry on rate limits, server errors, and network issues
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		errors.Is(err, context.DeadlineExceeded)
}

// retryWithBackoff executes fn with exponential backoff on retryable errors.
func retryWithBackoff[T any](ctx context.Context, logger *slog.Logger, operation string, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		result, lastErr = fn()
		if lastErr == nil {
			return result, nil
		}

		if !isRetryableError(lastErr) {
			return result, lastErr
		}

		if attempt < MaxRetries {
			delay := RetryBaseDelay * time.Duration(1<<attempt) // exponential backoff
			logger.Warn("retrying after transient error",
				"operation", operation,
				"attempt", attempt+1,
				"max_attempts", MaxRetries+1,
				"delay", delay,
				"error", lastErr,
			)

			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return result, fmt.Errorf("max retries exceeded for %s: %w", operation, lastErr)
}

// Reviewer orchestrates the code review process.
type Reviewer struct {
	githubClient   *github.Client
	configLoader   *config.Loader
	storage        storage.Storage
	claudeAPIKey   string // Default/fallback API key
	apiKeyFunc     APIKeyFunc
	modelFunc      ModelFunc
	model          string
	logger         *slog.Logger
	contextFetcher *ContextFetcher
}

// NewReviewer creates a new Reviewer instance.
func NewReviewer(githubClient *github.Client, claudeAPIKey string, store storage.Storage, logger *slog.Logger) *Reviewer {
	return &Reviewer{
		githubClient:   githubClient,
		configLoader:   config.NewLoader(githubClient),
		storage:        store,
		claudeAPIKey:   claudeAPIKey,
		model:          DefaultModel,
		logger:         logger,
		contextFetcher: NewContextFetcher(githubClient, logger),
	}
}

// SetAPIKeyFunc sets a function to resolve API keys per installation.
func (r *Reviewer) SetAPIKeyFunc(fn APIKeyFunc) {
	r.apiKeyFunc = fn
}

// SetModel overrides the default Claude model used for reviews.
func (r *Reviewer) SetModel(model string) {
	r.model = model
}

// SetModelFunc sets a function to resolve the Claude model per installation.
func (r *Reviewer) SetModelFunc(fn ModelFunc) {
	r.modelFunc = fn
}

// getModel returns the appropriate model for the installation.
// If a ModelFunc is set and returns a non-empty model, that takes priority.
// Otherwise, it returns the global model (set via SetModel or DefaultModel).
func (r *Reviewer) getModel(ctx context.Context, installationID int64) string {
	if r.modelFunc != nil {
		model, err := r.modelFunc(ctx, installationID)
		if err != nil {
			r.logger.Warn("ModelFunc failed, using default model", "error", err, "installation_id", installationID)
			return r.model
		}
		if model != "" {
			return model
		}
	}
	return r.model
}

// getAPIKey returns the appropriate API key for the installation.
// If an APIKeyFunc is set, it delegates to that function.
// Otherwise, it returns the default/global API key.
func (r *Reviewer) getAPIKey(ctx context.Context, installationID int64) (string, bool, error) {
	if r.apiKeyFunc != nil {
		apiKey, isCustomKey, err := r.apiKeyFunc(ctx, installationID)
		if err != nil {
			r.logger.Warn("APIKeyFunc failed, using default key", "error", err, "installation_id", installationID)
			return r.claudeAPIKey, false, nil
		}
		return apiKey, isCustomKey, nil
	}
	return r.claudeAPIKey, false, nil
}

// ReviewInput contains the information needed to review a pull request.
type ReviewInput struct {
	InstallationID int64
	Owner          string
	Repo           string
	PRNumber       int
	PRTitle        string
	PRBody         string
	HeadSHA        string
	DefaultBranch  string

}

// ReviewResult contains the result of a review.
type ReviewResult struct {
	ReviewID     int64
	ReviewURL    string
	Summary      string
	CommentCount int
	Approval     string
	Usage        *storage.TokenUsage
}

// ClaudeAPIResponse contains the raw text response and token usage from a Claude API call.
type ClaudeAPIResponse struct {
	Text  string
	Usage *storage.TokenUsage
}

// Review performs a code review on a pull request.
// It automatically detects whether this is the first review or a subsequent one
// and handles them appropriately.
func (r *Reviewer) Review(ctx context.Context, input *ReviewInput) (*ReviewResult, error) {
	r.logger.Info("starting review",
		"owner", input.Owner,
		"repo", input.Repo,
		"pr", input.PRNumber,
	)

	// Load repo config
	cfg, err := r.configLoader.Load(ctx, input.InstallationID, input.Owner, input.Repo, input.DefaultBranch)
	if err != nil {
		var parseErr *config.ConfigParseError
		if errors.As(err, &parseErr) {
			// Config file exists but has invalid content - this is a user error that should be surfaced
			r.logger.Error("invalid config file, cannot proceed with review",
				"path", parseErr.Path,
				"error", parseErr.Err,
			)
			return nil, fmt.Errorf("invalid config file %s: %w", parseErr.Path, parseErr.Err)
		}
		// Other errors (network issues, etc.) - use defaults and continue
		r.logger.Warn("failed to load config, using defaults", "error", err)
		cfg = config.DefaultConfig()
	}

	if !cfg.ShouldReviewOnEvent() {
		r.logger.Info("review skipped due to config",
			"enabled", cfg.Enabled,
			"trigger", cfg.Trigger,
		)
		return nil, nil
	}

	// Fetch diff
	diff, err := r.githubClient.FetchDiff(ctx, input.InstallationID, input.Owner, input.Repo, input.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch diff: %w", err)
	}

	r.logger.Info("fetched diff", "size", len(diff))

	// Filter diff based on exclude patterns
	if len(cfg.Exclude) > 0 {
		diff = filterDiff(diff, cfg)
		r.logger.Info("filtered diff", "size", len(diff), "exclude_patterns", cfg.Exclude)
	}

	// Get the appropriate API key
	apiKey, isCustomKey, err := r.getAPIKey(ctx, input.InstallationID)
	if err != nil {
		r.logger.Warn("failed to get API key, using default", "error", err)
		apiKey = r.claudeAPIKey
		isCustomKey = false
	}

	r.logger.Info("using API key", "is_custom_key", isCustomKey, "installation_id", input.InstallationID)

	// Resolve the model for this installation
	model := r.getModel(ctx, input.InstallationID)
	r.logger.Info("using model", "model", model, "installation_id", input.InstallationID)

	// Check if this is a subsequent review (we have a previous review stored)
	var firstReview *storage.ReviewContext
	if r.storage != nil {
		firstReview, err = r.storage.GetFirstReviewForPR(ctx, input.InstallationID, input.Owner, input.Repo, input.PRNumber)
		if err != nil {
			r.logger.Warn("failed to check for existing reviews, treating as first review", "error", err)
			// Continue as first review
		}
	}

	if firstReview != nil {
		r.logger.Info("detected subsequent review",
			"first_review_id", firstReview.ReviewID,
		)
		return r.reviewSubsequent(ctx, input, firstReview, cfg, diff, apiKey, model)
	}

	return r.reviewFirst(ctx, input, cfg, diff, apiKey, model)
}

// reviewFirst handles the first review of a PR (creates new review with inline comments).
func (r *Reviewer) reviewFirst(ctx context.Context, input *ReviewInput, cfg *config.Config, diff, apiKey, model string) (*ReviewResult, error) {
	r.logger.Info("performing first review")

	// Extract changed file paths from the diff
	diffInfo := ParseDiffInfo(diff)
	changedFiles := diffInfo.Files

	// Fetch rich context (full files, related files, commit history)
	var reviewCtx *ReviewContext
	if len(changedFiles) > 0 {
		contextInput := &ContextInput{
			InstallationID: input.InstallationID,
			Owner:          input.Owner,
			Repo:           input.Repo,
			HeadRef:        input.HeadSHA,
			ChangedFiles:   changedFiles,
			Config:         cfg,
		}
		reviewCtx = r.contextFetcher.FetchContext(ctx, contextInput)
		r.logger.Info("fetched review context",
			"full_files", len(reviewCtx.FullFiles),
			"related_files", len(reviewCtx.RelatedFiles),
			"file_histories", len(reviewCtx.FileHistories),
			"total_size", reviewCtx.TotalSize(),
		)
	}

	// Check if diff needs chunking
	var parsed *ClaudeResponse
	var totalUsage *storage.TokenUsage
	var err error

	if len(diff) > ChunkThreshold {
		r.logger.Info("diff exceeds chunk threshold, using chunked review",
			"diff_size", len(diff),
			"threshold", ChunkThreshold,
		)
		parsed, totalUsage, err = r.reviewChunked(ctx, apiKey, model, input, diff, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed chunked review: %w", err)
		}
	} else {
		// Standard single-call review with context
		claudeResp, err := r.callClaudeWithContext(ctx, apiKey, model, input.PRTitle, input.PRBody, diff, cfg.ClaudeMD, cfg.Instructions, reviewCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to get Claude review: %w", err)
		}

		parsed, err = ParseResponse(claudeResp.Text)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Claude response: %w", err)
		}
		totalUsage = claudeResp.Usage
	}

	r.logger.Info("parsed Claude response",
		"summary", parsed.Summary,
		"comments", len(parsed.Comments),
		"approval", parsed.Approval,
	)

	// Validate and filter comments against diff lines
	diffLines := ParseDiffLines(diff)
	parsed.Comments, _ = FilterValidComments(parsed.Comments, diffLines, r.logger)

	// Convert to GitHub review
	reviewReq, err := ToGitHubReview(parsed, input.HeadSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to GitHub review: %w", err)
	}

	// Post review to GitHub
	review, err := r.githubClient.CreateReview(ctx, input.InstallationID, input.Owner, input.Repo, input.PRNumber, reviewReq)
	if err != nil {
		return nil, fmt.Errorf("failed to post review: %w", err)
	}

	r.logger.Info("posted review", "review_id", review.ID, "url", review.HTMLURL)

	// Store review context (excluding raw Claude response to avoid retaining customer code)
	if r.storage != nil {
		storeCtx := &storage.ReviewContext{
			InstallationID: input.InstallationID,
			Owner:          input.Owner,
			Repo:           input.Repo,
			PRNumber:       input.PRNumber,
			ReviewID:       review.ID,
			ReviewBody:     parsed.Summary,
			Comments:       toStorageComments(parsed.Comments),
			Usage:          totalUsage,
			UsageType:      "review",
		}

		if err := r.storage.StoreReview(ctx, storeCtx); err != nil {
			r.logger.Error("failed to store review context", "error", err)
			// Don't fail the review if storage fails
		}

	}

	return &ReviewResult{
		ReviewID:     review.ID,
		ReviewURL:    review.HTMLURL,
		Summary:      parsed.Summary,
		CommentCount: len(parsed.Comments),
		Approval:     parsed.Approval,
		Usage:        totalUsage,
	}, nil
}

// reviewSubsequent handles subsequent reviews by updating the original review body
// and posting new comments separately.
func (r *Reviewer) reviewSubsequent(ctx context.Context, input *ReviewInput, firstReview *storage.ReviewContext, cfg *config.Config, diff, apiKey, model string) (*ReviewResult, error) {
	r.logger.Info("performing subsequent review",
		"first_review_id", firstReview.ReviewID,
	)

	// Fetch existing review threads with resolution status via GraphQL
	threads, err := r.githubClient.FetchPRReviewThreads(ctx, input.InstallationID, input.Owner, input.Repo, input.PRNumber)
	if err != nil {
		r.logger.Warn("failed to fetch review threads, falling back to first review behavior", "error", err)
		return r.reviewFirst(ctx, input, cfg, diff, apiKey, model)
	}

	// Convert threads to ExistingComment format for the prompt
	existingComments := convertThreadsToExistingComments(threads)
	r.logger.Info("fetched existing comments",
		"thread_count", len(threads),
		"comment_count", len(existingComments),
	)

	// Extract changed file paths from the diff
	diffInfo := ParseDiffInfo(diff)
	changedFiles := diffInfo.Files

	// Fetch rich context
	var reviewCtx *ReviewContext
	if len(changedFiles) > 0 {
		contextInput := &ContextInput{
			InstallationID: input.InstallationID,
			Owner:          input.Owner,
			Repo:           input.Repo,
			HeadRef:        input.HeadSHA,
			ChangedFiles:   changedFiles,
			Config:         cfg,
		}
		reviewCtx = r.contextFetcher.FetchContext(ctx, contextInput)
	}

	// Call Claude with subsequent review prompt
	claudeResp, err := r.callClaudeSubsequent(ctx, apiKey, model, input, diff, existingComments, cfg, reviewCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Claude subsequent review: %w", err)
	}

	parsed, err := ParseResponse(claudeResp.Text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Claude response: %w", err)
	}

	// Determine approval based on severity (only blockers trigger request_changes)
	parsed.Approval = DetermineApprovalFromSeverity(parsed.Comments)

	// Validate and filter comments against diff lines
	diffLines := ParseDiffLines(diff)
	parsed.Comments, _ = FilterValidComments(parsed.Comments, diffLines, r.logger)

	r.logger.Info("parsed subsequent review response",
		"summary", parsed.Summary,
		"new_comments", len(parsed.Comments),
		"approval", parsed.Approval,
	)

	// Build the updated summary that appends to the original
	newBody := buildConsolidatedSummary(firstReview.ReviewBody, parsed.Summary, input)

	// Update the original review's body
	if err := r.githubClient.UpdateReviewBody(ctx, input.InstallationID, input.Owner, input.Repo, input.PRNumber, firstReview.ReviewID, newBody); err != nil {
		r.logger.Error("failed to update original review body", "error", err)
		// Continue to post new comments even if summary update fails
	} else {
		r.logger.Info("updated original review body", "review_id", firstReview.ReviewID)
	}

	// If there are new comments, post them as a minimal COMMENT review
	var newReviewID int64
	var newReviewURL string
	if len(parsed.Comments) > 0 {
		reviewComments := make([]github.ReviewComment, len(parsed.Comments))
		for i, c := range parsed.Comments {
			reviewComments[i] = github.ReviewComment{
				Path: c.Path,
				Line: c.Line,
				Side: "RIGHT",
				Body: FormatCommentWithSeverity(c.Body, c.Severity),
			}
		}

		// Create a minimal COMMENT review (not APPROVE or REQUEST_CHANGES)
		// to attach the new inline comments
		reviewReq := &github.ReviewRequest{
			CommitID: input.HeadSHA,
			Body:     "", // Empty body since we updated the original
			Event:    "COMMENT",
			Comments: reviewComments,
		}

		newReview, err := r.githubClient.CreateReview(ctx, input.InstallationID, input.Owner, input.Repo, input.PRNumber, reviewReq)
		if err != nil {
			return nil, fmt.Errorf("failed to post new comments: %w", err)
		}
		newReviewID = newReview.ID
		newReviewURL = newReview.HTMLURL
		r.logger.Info("posted new comments", "review_id", newReview.ID, "comment_count", len(parsed.Comments))
	}

	// Store review context for this subsequent review
	if r.storage != nil {
		storeCtx := &storage.ReviewContext{
			InstallationID: input.InstallationID,
			Owner:          input.Owner,
			Repo:           input.Repo,
			PRNumber:       input.PRNumber,
			ReviewID:       newReviewID,
			ReviewBody:     parsed.Summary,
			Comments:       toStorageComments(parsed.Comments),
			Usage:          claudeResp.Usage,
			UsageType:      "review",
		}

		if err := r.storage.StoreReview(ctx, storeCtx); err != nil {
			r.logger.Error("failed to store review context", "error", err)
		}
	}

	// Return the first review ID since that's the main review we're updating
	return &ReviewResult{
		ReviewID:     firstReview.ReviewID,
		ReviewURL:    newReviewURL,
		Summary:      parsed.Summary,
		CommentCount: len(parsed.Comments),
		Approval:     parsed.Approval,
		Usage:        claudeResp.Usage,
	}, nil
}

// callClaudeSubsequent sends the subsequent review request to Claude.
func (r *Reviewer) callClaudeSubsequent(ctx context.Context, apiKey, model string, input *ReviewInput, diff string, existingComments []ExistingComment, cfg *config.Config, reviewCtx *ReviewContext) (*ClaudeAPIResponse, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// Build prompt with existing comments context
	prompt := BuildSubsequentReviewPrompt(input.PRTitle, input.PRBody, diff, existingComments)

	// Add rich context if available
	if reviewCtx != nil && !reviewCtx.IsEmpty() {
		prompt = formatContext(reviewCtx) + "\n\n---\n\n" + prompt
	}

	// Add timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, ClaudeAPITimeout)
	defer cancel()

	// Retry on transient failures
	message, err := retryWithBackoff(timeoutCtx, r.logger, "callClaudeSubsequent", func() (*anthropic.Message, error) {
		return client.Messages.New(timeoutCtx, anthropic.MessageNewParams{
			Model:     anthropic.F(anthropic.Model(model)),
			MaxTokens: anthropic.F(int64(4096)),
			System: anthropic.F([]anthropic.TextBlockParam{
				anthropic.NewTextBlock(GetSubsequentReviewSystemPrompt(cfg.ClaudeMD, cfg.Instructions)),
			}),
			Messages: anthropic.F([]anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("Claude API error: %w", err)
	}

	// Capture token usage
	usage := &storage.TokenUsage{
		InputTokens:              message.Usage.InputTokens,
		OutputTokens:             message.Usage.OutputTokens,
		CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
		CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
	}
	r.logger.Info("Claude API usage (subsequent)",
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
	)

	// Extract text from response
	for _, block := range message.Content {
		if block.Type == anthropic.ContentBlockTypeText {
			return &ClaudeAPIResponse{
				Text:  block.Text,
				Usage: usage,
			}, nil
		}
	}

	return nil, fmt.Errorf("no text content in Claude response")
}

// convertThreadsToExistingComments converts GitHub review threads to ExistingComment format.
func convertThreadsToExistingComments(threads []github.ReviewThread) []ExistingComment {
	var comments []ExistingComment
	for _, t := range threads {
		// Use the first comment in each thread as the main comment
		if len(t.Comments) > 0 {
			c := t.Comments[0]
			comments = append(comments, ExistingComment{
				Path:       t.Path,
				Line:       t.Line,
				Body:       c.Body,
				IsResolved: t.IsResolved,
				Author:     c.Author,
			})
		}
	}
	return comments
}

// buildConsolidatedSummary creates an updated summary that appends to the original.
func buildConsolidatedSummary(originalSummary, newSummary string, input *ReviewInput) string {
	timestamp := time.Now().UTC().Format("Jan 2, 2006 15:04 UTC")
	shortSHA := input.HeadSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	return fmt.Sprintf("%s\n\n---\n\n**Update (%s, commit %s):** %s", originalSummary, timestamp, shortSHA, newSummary)
}

// callClaudeWithContext sends the review request to Claude with optional rich context.
func (r *Reviewer) callClaudeWithContext(ctx context.Context, apiKey, model, title, description, diff, claudeMD, instructions string, reviewCtx *ReviewContext) (*ClaudeAPIResponse, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// Build prompt with or without context
	var prompt string
	hasContext := reviewCtx != nil && !reviewCtx.IsEmpty()
	if hasContext {
		prompt = BuildPromptWithContext(title, description, diff, reviewCtx)
	} else {
		prompt = BuildPrompt(title, description, diff)
	}

	// Add timeout to prevent hanging indefinitely
	timeoutCtx, cancel := context.WithTimeout(ctx, ClaudeAPITimeout)
	defer cancel()

	// Retry on transient failures
	message, err := retryWithBackoff(timeoutCtx, r.logger, "callClaude", func() (*anthropic.Message, error) {
		return client.Messages.New(timeoutCtx, anthropic.MessageNewParams{
			Model:     anthropic.F(anthropic.Model(model)),
			MaxTokens: anthropic.F(int64(4096)),
			System: anthropic.F([]anthropic.TextBlockParam{
				anthropic.NewTextBlock(GetSystemPromptWithContext(claudeMD, instructions, hasContext)),
			}),
			Messages: anthropic.F([]anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("Claude API error: %w", err)
	}

	// Capture token usage
	usage := &storage.TokenUsage{
		InputTokens:              message.Usage.InputTokens,
		OutputTokens:             message.Usage.OutputTokens,
		CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
		CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
	}
	r.logger.Info("Claude API usage",
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
		"cache_read_tokens", usage.CacheReadInputTokens,
	)

	// Extract text from response
	for _, block := range message.Content {
		if block.Type == anthropic.ContentBlockTypeText {
			return &ClaudeAPIResponse{
				Text:  block.Text,
				Usage: usage,
			}, nil
		}
	}

	return nil, fmt.Errorf("no text content in Claude response")
}

// reviewChunked handles large diffs by splitting them into chunks and reviewing in parallel.
func (r *Reviewer) reviewChunked(ctx context.Context, apiKey, model string, input *ReviewInput, diff string, cfg *config.Config) (*ClaudeResponse, *storage.TokenUsage, error) {
	chunks := ChunkDiff(diff, MaxChunkSize)

	r.logger.Info("chunked diff",
		"chunk_count", len(chunks),
		"diff_size", len(diff),
	)

	if len(chunks) == 0 {
		// Return empty usage instead of nil for consistent tracking
		return &ClaudeResponse{
			Summary:  "No content to review.",
			Approval: "comment",
		}, &storage.TokenUsage{}, nil
	}

	// Prepare context input for per-chunk fetching
	contextInput := &ContextInput{
		InstallationID: input.InstallationID,
		Owner:          input.Owner,
		Repo:           input.Repo,
		HeadRef:        input.HeadSHA,
		Config:         cfg,
	}

	// Process chunks in parallel using errgroup with concurrency limit
	g, gctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(MaxConcurrentChunks)
	results := make([]*ChunkResult, len(chunks))
	usages := make([]*storage.TokenUsage, len(chunks))
	var usageMu sync.Mutex

	for i, chunk := range chunks {
		i, chunk := i, chunk // capture for goroutine
		g.Go(func() error {
			// Acquire semaphore to limit concurrency
			if err := sem.Acquire(gctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)

			// Get files for this chunk
			chunkFiles := make([]string, len(chunk.Files))
			for j, f := range chunk.Files {
				chunkFiles[j] = f.Path
			}

			// Fetch context for this chunk
			chunkCtx := r.contextFetcher.FetchContextForChunk(gctx, contextInput, chunkFiles, i, len(chunks))

			resp, usage, err := r.reviewChunkWithContext(gctx, apiKey, model, input, &chunk, cfg, chunkCtx)
			if err != nil {
				return fmt.Errorf("chunk %d/%d: %w", i+1, len(chunks), err)
			}
			results[i] = &ChunkResult{
				Response: resp,
				Index:    i,
			}
			usageMu.Lock()
			usages[i] = usage
			usageMu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	// Merge results
	merged, err := MergeChunkResponses(results)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to merge chunk responses: %w", err)
	}

	// Aggregate token usage
	totalUsage := aggregateUsage(usages)

	r.logger.Info("merged chunked review",
		"total_comments", len(merged.Comments),
		"approval", merged.Approval,
		"total_input_tokens", totalUsage.InputTokens,
		"total_output_tokens", totalUsage.OutputTokens,
	)

	return &ClaudeResponse{
		Summary:  merged.Summary,
		Comments: merged.Comments,
		Approval: merged.Approval,
	}, totalUsage, nil
}


// reviewChunkWithContext reviews a single chunk with optional rich context.
func (r *Reviewer) reviewChunkWithContext(ctx context.Context, apiKey, model string, input *ReviewInput, chunk *Chunk, cfg *config.Config, reviewCtx *ReviewContext) (*ClaudeResponse, *storage.TokenUsage, error) {
	// Extract file paths for the prompt
	filePaths := make([]string, len(chunk.Files))
	for i, f := range chunk.Files {
		filePaths[i] = f.Path
	}

	diff := ChunkToDiff(chunk)

	r.logger.Info("reviewing chunk",
		"chunk", chunk.Index+1,
		"total", chunk.Total,
		"files", len(chunk.Files),
		"size", len(diff),
	)

	// Build chunked prompt with or without context
	var prompt string
	hasContext := reviewCtx != nil && !reviewCtx.IsEmpty()
	if hasContext {
		prompt = BuildChunkedPromptWithContext(input.PRTitle, input.PRBody, diff, chunk.Index, chunk.Total, filePaths, reviewCtx)
	} else {
		prompt = BuildChunkedPrompt(input.PRTitle, input.PRBody, diff, chunk.Index, chunk.Total, filePaths)
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// Add timeout to prevent hanging indefinitely
	timeoutCtx, cancel := context.WithTimeout(ctx, ClaudeAPITimeout)
	defer cancel()

	// Retry on transient failures
	message, err := retryWithBackoff(timeoutCtx, r.logger, fmt.Sprintf("reviewChunk_%d", chunk.Index+1), func() (*anthropic.Message, error) {
		return client.Messages.New(timeoutCtx, anthropic.MessageNewParams{
			Model:     anthropic.F(anthropic.Model(model)),
			MaxTokens: anthropic.F(int64(4096)),
			System: anthropic.F([]anthropic.TextBlockParam{
				anthropic.NewTextBlock(GetSystemPromptWithContext(cfg.ClaudeMD, cfg.Instructions, hasContext)),
			}),
			Messages: anthropic.F([]anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			}),
		})
	})
	if err != nil {
		return nil, nil, fmt.Errorf("Claude API error: %w", err)
	}

	// Capture token usage
	usage := &storage.TokenUsage{
		InputTokens:              message.Usage.InputTokens,
		OutputTokens:             message.Usage.OutputTokens,
		CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
		CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
	}
	r.logger.Info("chunk Claude API usage",
		"chunk", chunk.Index+1,
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
	)

	// Extract text from response
	var text string
	for _, block := range message.Content {
		if block.Type == anthropic.ContentBlockTypeText {
			text = block.Text
			break
		}
	}

	if text == "" {
		return nil, nil, fmt.Errorf("no text content in Claude response for chunk %d", chunk.Index+1)
	}

	// Parse the response
	parsed, err := ParseResponse(text)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse chunk %d response: %w", chunk.Index+1, err)
	}

	// Validate and filter comments against this chunk's diff lines
	diffLines := ParseDiffLines(diff)
	parsed.Comments, _ = FilterValidComments(parsed.Comments, diffLines, r.logger)

	return parsed, usage, nil
}

// aggregateUsage combines token usage from multiple chunks.
func aggregateUsage(usages []*storage.TokenUsage) *storage.TokenUsage {
	total := &storage.TokenUsage{}
	for _, u := range usages {
		if u == nil {
			continue
		}
		total.InputTokens += u.InputTokens
		total.OutputTokens += u.OutputTokens
		total.CacheReadInputTokens += u.CacheReadInputTokens
		total.CacheCreationInputTokens += u.CacheCreationInputTokens
	}
	return total
}

// toStorageComments converts ClaudeComments to storage comments.
func toStorageComments(comments []ClaudeComment) []storage.Comment {
	result := make([]storage.Comment, len(comments))
	for i, c := range comments {
		result[i] = storage.Comment{
			Path: c.Path,
			Line: c.Line,
			Body: c.Body,
		}
	}
	return result
}

// filterDiff removes files matching exclude patterns from the diff.
func filterDiff(diff string, cfg *config.Config) string {
	var result strings.Builder
	var currentFile string
	var includeFile bool
	var fileContent strings.Builder

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		// Detect new file in diff
		if strings.HasPrefix(line, "diff --git") {
			// Write previous file if it was included
			if includeFile && fileContent.Len() > 0 {
				result.WriteString(fileContent.String())
			}
			fileContent.Reset()

			// Extract file path from "diff --git a/path b/path"
			parts := strings.Split(line, " ")
			if len(parts) >= 4 {
				currentFile = strings.TrimPrefix(parts[3], "b/")
			}
			includeFile = !cfg.ShouldExcludeFile(currentFile)
		}

		if includeFile {
			fileContent.WriteString(line)
			fileContent.WriteString("\n")
		}
	}

	// Write last file if included
	if includeFile && fileContent.Len() > 0 {
		result.WriteString(fileContent.String())
	}

	return strings.TrimSuffix(result.String(), "\n")
}
