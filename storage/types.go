package storage

// Installation represents a GitHub App installation.
type Installation struct {
	InstallationID int64  `json:"installation_id"`
	AccountID      int64  `json:"account_id,omitempty"`
	OrgLogin       string `json:"org_login"`
	InstalledAt    string `json:"installed_at"`
	InstalledBy    string `json:"installed_by"`
}

// Comment represents a review comment for storage.
type Comment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
}

// TokenUsage represents Claude API token usage for a single call.
type TokenUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`
}

// ReviewContext represents the stored context for a review.
type ReviewContext struct {
	InstallationID int64       `json:"installation_id"`
	Owner          string      `json:"owner"`
	Repo           string      `json:"repo"`
	PRNumber       int         `json:"pr_number"`
	ReviewID       int64       `json:"review_id"`
	ReviewBody     string      `json:"review_body"`
	Comments       []Comment   `json:"comments"`
	CreatedAt      string      `json:"created_at"`
	Usage          *TokenUsage `json:"usage,omitempty"`
	UsageType      string      `json:"usage_type,omitempty"`
}
