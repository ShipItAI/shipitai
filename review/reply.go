package review

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/shipitai/shipitai/github"
	"github.com/shipitai/shipitai/storage"
)

const replySystemPrompt = `You are a helpful code review assistant. You previously reviewed a pull request and left comments. A developer is now asking you a follow-up question about one of your comments.

Be helpful, concise, and specific. If asked to clarify, provide concrete examples. If asked to reconsider, evaluate their argument fairly and either:
1. Acknowledge their point and withdraw your concern
2. Explain why you still think the issue is worth addressing

When you have a specific code fix to suggest, use GitHub's suggestion syntax so the author can apply it with one click:

` + "```suggestion" + `
fixed code here
` + "```" + `

IMPORTANT: The suggestion replaces ONLY the single line being discussed. Only include the replacement for that ONE line, not surrounding context. If the fix spans multiple existing lines, describe it in text instead.

Keep responses short and focused - this is a code review conversation, not an essay.`

const replyPromptTemplate = `The developer is asking about code in this file: %s

Here's the relevant code context (diff hunk):
%s

Here's the conversation thread:
%s

The developer's latest message:
%s

Respond helpfully and concisely.`

// ReplyInput contains the information needed to reply to a comment.
type ReplyInput struct {
	InstallationID int64
	Owner          string
	Repo           string
	PRNumber       int
	CommentID      int64
	DiffHunk       string
	FilePath       string
	UserQuestion   string
	ThreadContext  string // Previous comments in the thread
}

// ReplyResult contains the result of a reply.
type ReplyResult struct {
	CommentID  int64
	CommentURL string
	Body       string
	Usage      *storage.TokenUsage
}

// Reply responds to a user's comment that mentioned the bot.
func (r *Reviewer) Reply(ctx context.Context, input *ReplyInput) (*ReplyResult, error) {
	r.logger.Info("generating reply",
		"owner", input.Owner,
		"repo", input.Repo,
		"pr", input.PRNumber,
		"comment_id", input.CommentID,
	)

	// Get the appropriate API key
	apiKey, isCustomKey, err := r.getAPIKey(ctx, input.InstallationID)
	if err != nil {
		r.logger.Warn("failed to get API key for reply, using default", "error", err)
		apiKey = r.claudeAPIKey
		isCustomKey = false
	}

	r.logger.Info("using API key for reply", "is_custom_key", isCustomKey, "installation_id", input.InstallationID)

	// Generate reply using Claude
	claudeResp, err := r.generateReply(ctx, apiKey, input)
	if err != nil {
		return nil, fmt.Errorf("failed to generate reply: %w", err)
	}

	r.logger.Info("generated reply", "length", len(claudeResp.Text))

	// Post the reply
	comment, err := r.githubClient.CreateReplyComment(
		ctx,
		input.InstallationID,
		input.Owner,
		input.Repo,
		input.PRNumber,
		input.CommentID,
		claudeResp.Text,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to post reply: %w", err)
	}

	return &ReplyResult{
		CommentID:  comment.ID,
		CommentURL: comment.HTMLURL,
		Body:       claudeResp.Text,
		Usage:      claudeResp.Usage,
	}, nil
}

// generateReply calls Claude to generate a reply and returns usage info.
func (r *Reviewer) generateReply(ctx context.Context, apiKey string, input *ReplyInput) (*ClaudeAPIResponse, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	prompt := fmt.Sprintf(replyPromptTemplate,
		input.FilePath,
		input.DiffHunk,
		input.ThreadContext,
		input.UserQuestion,
	)

	// Add timeout to prevent hanging indefinitely
	timeoutCtx, cancel := context.WithTimeout(ctx, ClaudeAPITimeout)
	defer cancel()

	// Retry on transient failures
	message, err := retryWithBackoff(timeoutCtx, r.logger, "generateReply", func() (*anthropic.Message, error) {
		return client.Messages.New(timeoutCtx, anthropic.MessageNewParams{
			Model:     anthropic.F(anthropic.Model("claude-sonnet-4-20250514")),
			MaxTokens: anthropic.F(int64(1024)),
			System: anthropic.F([]anthropic.TextBlockParam{
				anthropic.NewTextBlock(replySystemPrompt),
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
	r.logger.Info("Claude API usage (reply)",
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

// BuildThreadContext builds the conversation context from a list of comments.
func BuildThreadContext(comments []github.PullRequestComment, targetCommentID int64) string {
	// Find the thread by tracing in_reply_to_id
	threadComments := findThreadComments(comments, targetCommentID)

	var sb strings.Builder
	for _, c := range threadComments {
		sb.WriteString(fmt.Sprintf("%s:\n%s\n\n", c.User.Login, c.Body))
	}

	return sb.String()
}

// findThreadComments finds all comments in a thread leading up to the target comment.
func findThreadComments(comments []github.PullRequestComment, targetID int64) []github.PullRequestComment {
	// Build a map for quick lookup
	commentMap := make(map[int64]*github.PullRequestComment)
	for i := range comments {
		commentMap[comments[i].ID] = &comments[i]
	}

	// Find the root of the thread and collect all comments
	var thread []github.PullRequestComment

	// First, find the root (comment with no in_reply_to_id or the original comment)
	current := commentMap[targetID]
	if current == nil {
		return thread
	}

	// Walk up to find the root
	rootID := targetID
	for current != nil && current.InReplyToID != 0 {
		rootID = current.InReplyToID
		current = commentMap[rootID]
	}

	// Now collect all comments in the thread (those that reply to root or its descendants)
	threadIDs := make(map[int64]bool)
	threadIDs[rootID] = true

	// Multiple passes to catch nested replies
	changed := true
	for changed {
		changed = false
		for _, c := range comments {
			if threadIDs[c.InReplyToID] && !threadIDs[c.ID] {
				threadIDs[c.ID] = true
				changed = true
			}
		}
	}

	// Collect comments in the thread (excluding the target)
	for _, c := range comments {
		if threadIDs[c.ID] && c.ID != targetID {
			thread = append(thread, c)
		}
	}

	// Sort by ID to ensure chronological order
	sort.Slice(thread, func(i, j int) bool {
		return thread[i].ID < thread[j].ID
	})

	return thread
}
