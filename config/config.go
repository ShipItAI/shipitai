// Package config handles loading and parsing repository configuration.
package config

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shipitai/shipitai/github"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultConfigPath is the default path for the shipitai config file.
	DefaultConfigPath = ".github/shipitai.yml"

	// TriggerAuto triggers a review automatically on PR events.
	TriggerAuto = "auto"
	// TriggerOnRequest triggers a review only when requested.
	TriggerOnRequest = "on-request"
)

// ConfigParseError indicates a configuration file exists but contains invalid content.
// This is distinct from "file not found" errors, which should use default config.
type ConfigParseError struct {
	Path string
	Err  error
}

func (e *ConfigParseError) Error() string {
	return fmt.Sprintf("invalid config at %s: %v", e.Path, e.Err)
}

func (e *ConfigParseError) Unwrap() error {
	return e.Err
}

// Config represents the repository configuration for the reviewer.
type Config struct {
	// Enabled determines if the reviewer is enabled for this repository.
	Enabled bool `yaml:"enabled"`
	// Trigger determines when reviews are triggered.
	// Valid values: "auto", "on-request"
	Trigger string `yaml:"trigger"`
	// Exclude is a list of glob patterns for files to skip during review.
	// Example: ["vendor/**", "*.gen.go", "docs/**"]
	Exclude []string `yaml:"exclude"`
	// Instructions provides custom guidance for the reviewer.
	// Example: "Focus on security. We use sqlc for DB queries."
	Instructions string `yaml:"instructions"`
	// Context configures rich context fetching for reviews.
	// If nil, defaults are used (all enabled).
	Context *ContextConfig `yaml:"context,omitempty"`
	// ContributorProtection restricts auto-reviews to repository contributors only.
	// Non-contributors can have their PRs reviewed when a contributor comments "@shipitai review".
	// If nil, defaults to true (protection enabled).
	ContributorProtection *bool `yaml:"contributor_protection,omitempty"`
	// ClaudeMD contains the contents of the repository's CLAUDE.md file.
	// This provides project-specific context for code reviews.
	ClaudeMD string `yaml:"-"`
}

// IsContributorProtectionEnabled returns true if contributor protection is enabled.
// Defaults to true if not explicitly set.
func (c *Config) IsContributorProtectionEnabled() bool {
	if c.ContributorProtection == nil {
		return true // Default: enabled
	}
	return *c.ContributorProtection
}

// ContextConfig configures the rich context feature for reviews.
type ContextConfig struct {
	// Enabled controls whether rich context is fetched at all.
	// If nil, defaults to true.
	Enabled *bool `yaml:"enabled,omitempty"`
	// FullFiles controls whether full file content is fetched.
	// If nil, defaults to true.
	FullFiles *bool `yaml:"full_files,omitempty"`
	// RelatedFiles controls whether test files and imports are fetched.
	// If nil, defaults to true.
	RelatedFiles *bool `yaml:"related_files,omitempty"`
	// History controls whether commit history is fetched.
	// If nil, defaults to true.
	History *bool `yaml:"history,omitempty"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled: true,
		Trigger: TriggerAuto,
	}
}

// Loader loads configuration from repositories.
type Loader struct {
	client *github.Client
}

// NewLoader creates a new config loader.
func NewLoader(client *github.Client) *Loader {
	return &Loader{client: client}
}

// Load fetches and parses the config from a repository.
// If the config file doesn't exist, returns the default config.
// If the config file exists but is invalid, returns a ConfigParseError.
// Also fetches CLAUDE.md if present (checks root first, then .github/).
func (l *Loader) Load(ctx context.Context, installationID int64, owner, repo, ref string) (*Config, error) {
	content, err := l.client.FetchFileContent(ctx, installationID, owner, repo, DefaultConfigPath, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}

	var config *Config
	if content == "" {
		config = DefaultConfig()
	} else {
		config, err = Parse([]byte(content))
		if err != nil {
			// Wrap parse errors so callers can distinguish from fetch errors
			return nil, &ConfigParseError{Path: DefaultConfigPath, Err: err}
		}
	}

	// Fetch CLAUDE.md for project context (try root first, then .github/)
	claudeMD, err := l.client.FetchFileContent(ctx, installationID, owner, repo, "CLAUDE.md", ref)
	if err != nil {
		// Log but don't fail - CLAUDE.md is optional
		claudeMD = ""
	}
	if claudeMD == "" {
		claudeMD, _ = l.client.FetchFileContent(ctx, installationID, owner, repo, ".github/CLAUDE.md", ref)
	}
	config.ClaudeMD = claudeMD

	return config, nil
}

// Parse parses a config from YAML content.
func Parse(content []byte) (*Config, error) {
	config := DefaultConfig()
	if err := yaml.Unmarshal(content, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	switch c.Trigger {
	case TriggerAuto, TriggerOnRequest, "":
		// Valid values
		if c.Trigger == "" {
			c.Trigger = TriggerAuto
		}
	default:
		return fmt.Errorf("invalid trigger value: %s (must be 'auto' or 'on-request')", c.Trigger)
	}

	return nil
}

// ShouldReviewOnEvent returns true if a review should be triggered for automatic events.
func (c *Config) ShouldReviewOnEvent() bool {
	return c.Enabled && c.Trigger == TriggerAuto
}

// ShouldExcludeFile returns true if the file path matches any exclude pattern.
func (c *Config) ShouldExcludeFile(path string) bool {
	for _, pattern := range c.Exclude {
		// Handle ** patterns by checking if any path segment matches
		if strings.Contains(pattern, "**") {
			// Convert ** pattern to check directory prefix
			prefix := strings.Split(pattern, "**")[0]
			if prefix != "" && strings.HasPrefix(path, prefix) {
				// Check suffix if present
				suffix := strings.Split(pattern, "**")[1]
				if suffix == "" || strings.HasSuffix(path, strings.TrimPrefix(suffix, "/")) {
					return true
				}
			}
			// Also try matching without ** (e.g., "vendor/**" matches "vendor/foo.go")
			if prefix != "" && strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")) {
				return true
			}
		}

		// Standard glob matching
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}

		// Also try matching just the filename for patterns like "*.gen.go"
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}
