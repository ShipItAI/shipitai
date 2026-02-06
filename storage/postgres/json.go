package postgres

import (
	"encoding/json"

	"github.com/shipitai/shipitai/storage"
)

// commentsToJSON converts comments to a JSON string for storage.
func commentsToJSON(comments []storage.Comment) string {
	if len(comments) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(comments)
	return string(b)
}

// commentsFromJSON parses a JSON string into comments.
func commentsFromJSON(s string) []storage.Comment {
	if s == "" || s == "null" {
		return nil
	}
	var comments []storage.Comment
	if err := json.Unmarshal([]byte(s), &comments); err != nil {
		return nil
	}
	return comments
}

// usageToJSON converts token usage to a JSON string for storage.
func usageToJSON(usage *storage.TokenUsage) string {
	if usage == nil {
		return "null"
	}
	b, _ := json.Marshal(usage)
	return string(b)
}

// usageFromJSON parses a JSON string into token usage.
func usageFromJSON(s string) *storage.TokenUsage {
	if s == "" || s == "null" {
		return nil
	}
	var usage storage.TokenUsage
	if err := json.Unmarshal([]byte(s), &usage); err != nil {
		return nil
	}
	return &usage
}
