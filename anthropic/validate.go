// Package anthropic provides Anthropic API utilities.
package anthropic

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ValidateAPIKey validates an Anthropic API key by making a minimal API call.
// Returns nil if the key is valid, or an error describing the problem.
func ValidateAPIKey(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key is empty")
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// Make a minimal API call to verify the key works
	// Using Haiku with max 1 token to minimize cost
	_, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaude3_5HaikuLatest),
		MaxTokens: anthropic.F(int64(1)),
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		}),
	})
	if err != nil {
		return fmt.Errorf("API key validation failed: %w", err)
	}

	return nil
}

// ExtractKeyHint returns the last 4 characters of an API key for display purposes.
func ExtractKeyHint(apiKey string) string {
	if len(apiKey) < 4 {
		return "****"
	}
	return apiKey[len(apiKey)-4:]
}
