package config

import (
	"fmt"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
		check   func(*Config) error
	}{
		{
			name:    "valid config",
			content: "enabled: true\ntrigger: auto",
			wantErr: false,
			check: func(c *Config) error {
				if !c.Enabled {
					t.Error("Enabled should be true")
				}
				if c.Trigger != TriggerAuto {
					t.Errorf("Trigger = %v, want %v", c.Trigger, TriggerAuto)
				}
				return nil
			},
		},
		{
			name:    "on-request trigger",
			content: "enabled: true\ntrigger: on-request",
			wantErr: false,
			check: func(c *Config) error {
				if c.Trigger != TriggerOnRequest {
					t.Errorf("Trigger = %v, want %v", c.Trigger, TriggerOnRequest)
				}
				return nil
			},
		},
		{
			name:    "disabled",
			content: "enabled: false",
			wantErr: false,
			check: func(c *Config) error {
				if c.Enabled {
					t.Error("Enabled should be false")
				}
				return nil
			},
		},
		{
			name:    "empty trigger defaults to auto",
			content: "enabled: true",
			wantErr: false,
			check: func(c *Config) error {
				if c.Trigger != TriggerAuto {
					t.Errorf("Trigger = %v, want %v", c.Trigger, TriggerAuto)
				}
				return nil
			},
		},
		{
			name:    "invalid trigger",
			content: "enabled: true\ntrigger: invalid",
			wantErr: true,
		},
		{
			name:    "invalid YAML",
			content: "enabled: [invalid",
			wantErr: true,
		},
		{
			name:    "with exclude patterns",
			content: "enabled: true\nexclude:\n  - vendor/**\n  - \"*.gen.go\"",
			wantErr: false,
			check: func(c *Config) error {
				if len(c.Exclude) != 2 {
					t.Errorf("Exclude length = %v, want 2", len(c.Exclude))
				}
				if c.Exclude[0] != "vendor/**" {
					t.Errorf("Exclude[0] = %v, want vendor/**", c.Exclude[0])
				}
				return nil
			},
		},
		{
			name:    "with instructions",
			content: "enabled: true\ninstructions: Focus on security",
			wantErr: false,
			check: func(c *Config) error {
				if c.Instructions != "Focus on security" {
					t.Errorf("Instructions = %v, want 'Focus on security'", c.Instructions)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := Parse([]byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				if err := tt.check(config); err != nil {
					t.Errorf("check() failed: %v", err)
				}
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("Default Enabled should be true")
	}
	if config.Trigger != TriggerAuto {
		t.Errorf("Default Trigger = %v, want %v", config.Trigger, TriggerAuto)
	}
}

func TestShouldReviewOnEvent(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "enabled with auto trigger",
			config: &Config{Enabled: true, Trigger: TriggerAuto},
			want:   true,
		},
		{
			name:   "enabled with on-request trigger",
			config: &Config{Enabled: true, Trigger: TriggerOnRequest},
			want:   false,
		},
		{
			name:   "disabled with auto trigger",
			config: &Config{Enabled: false, Trigger: TriggerAuto},
			want:   false,
		},
		{
			name:   "disabled with on-request trigger",
			config: &Config{Enabled: false, Trigger: TriggerOnRequest},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.ShouldReviewOnEvent(); got != tt.want {
				t.Errorf("ShouldReviewOnEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldExcludeFile(t *testing.T) {
	tests := []struct {
		name    string
		exclude []string
		path    string
		want    bool
	}{
		{
			name:    "no patterns",
			exclude: nil,
			path:    "src/main.go",
			want:    false,
		},
		{
			name:    "vendor directory match",
			exclude: []string{"vendor/**"},
			path:    "vendor/github.com/foo/bar.go",
			want:    true,
		},
		{
			name:    "vendor root match",
			exclude: []string{"vendor/**"},
			path:    "vendor/foo.go",
			want:    true,
		},
		{
			name:    "non-vendor path",
			exclude: []string{"vendor/**"},
			path:    "src/vendor/fake.go",
			want:    false,
		},
		{
			name:    "generated file extension",
			exclude: []string{"*.gen.go"},
			path:    "internal/types.gen.go",
			want:    true,
		},
		{
			name:    "non-generated file",
			exclude: []string{"*.gen.go"},
			path:    "internal/types.go",
			want:    false,
		},
		{
			name:    "docs directory",
			exclude: []string{"docs/**"},
			path:    "docs/api/readme.md",
			want:    true,
		},
		{
			name:    "multiple patterns first match",
			exclude: []string{"vendor/**", "*.gen.go", "docs/**"},
			path:    "vendor/lib.go",
			want:    true,
		},
		{
			name:    "multiple patterns second match",
			exclude: []string{"vendor/**", "*.gen.go", "docs/**"},
			path:    "api/types.gen.go",
			want:    true,
		},
		{
			name:    "multiple patterns no match",
			exclude: []string{"vendor/**", "*.gen.go", "docs/**"},
			path:    "src/main.go",
			want:    false,
		},
		{
			name:    "exact filename pattern",
			exclude: []string{"go.sum"},
			path:    "go.sum",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Exclude: tt.exclude}
			if got := cfg.ShouldExcludeFile(tt.path); got != tt.want {
				t.Errorf("ShouldExcludeFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestConfigParseError(t *testing.T) {
	t.Run("error message includes path and underlying error", func(t *testing.T) {
		underlying := fmt.Errorf("yaml: line 1: could not find expected ':'")
		parseErr := &ConfigParseError{
			Path: ".github/shipitai.yml",
			Err:  underlying,
		}

		errMsg := parseErr.Error()
		if errMsg != "invalid config at .github/shipitai.yml: yaml: line 1: could not find expected ':'" {
			t.Errorf("Error() = %q, want message containing path and underlying error", errMsg)
		}
	})

	t.Run("errors.Is works with Unwrap", func(t *testing.T) {
		underlying := fmt.Errorf("some parse error")
		parseErr := &ConfigParseError{
			Path: ".github/shipitai.yml",
			Err:  underlying,
		}

		if parseErr.Unwrap() != underlying {
			t.Error("Unwrap() should return underlying error")
		}
	})
}

func TestIsContributorProtectionEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "nil defaults to true",
			config: &Config{},
			want:   true,
		},
		{
			name:   "explicitly enabled",
			config: &Config{ContributorProtection: boolPtr(true)},
			want:   true,
		},
		{
			name:   "explicitly disabled",
			config: &Config{ContributorProtection: boolPtr(false)},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsContributorProtectionEnabled(); got != tt.want {
				t.Errorf("IsContributorProtectionEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
