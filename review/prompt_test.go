package review

import (
	"strings"
	"testing"
)

func TestBuildSubsequentReviewPrompt(t *testing.T) {
	tests := []struct {
		name             string
		title            string
		description      string
		diff             string
		existingComments []ExistingComment
		wantContains     []string
	}{
		{
			name:        "includes existing comments",
			title:       "Test PR",
			description: "Test description",
			diff:        "diff --git a/main.go b/main.go\n+line",
			existingComments: []ExistingComment{
				{
					Path:       "main.go",
					Line:       42,
					Body:       "Consider error handling",
					IsResolved: false,
					Author:     "reviewer",
				},
			},
			wantContains: []string{
				"[UNRESOLVED]",
				"main.go:42",
				"@reviewer",
				"Consider error handling",
				"SUBSEQUENT REVIEW",
			},
		},
		{
			name:        "shows resolved status",
			title:       "Test PR",
			description: "Test description",
			diff:        "diff --git a/main.go b/main.go\n+line",
			existingComments: []ExistingComment{
				{
					Path:       "util.go",
					Line:       10,
					Body:       "Fixed this",
					IsResolved: true,
					Author:     "other",
				},
			},
			wantContains: []string{
				"[RESOLVED]",
				"util.go:10",
			},
		},
		{
			name:             "no previous comments",
			title:            "Test PR",
			description:      "Test description",
			diff:             "diff --git a/main.go b/main.go\n+line",
			existingComments: []ExistingComment{},
			wantContains: []string{
				"(No previous comments)",
			},
		},
		{
			name:        "includes severity instructions",
			title:       "Test PR",
			description: "Test description",
			diff:        "diff",
			existingComments: []ExistingComment{},
			wantContains: []string{
				"blocker",
				"suggestion",
				"nitpick",
				"severity",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSubsequentReviewPrompt(tt.title, tt.description, tt.diff, tt.existingComments)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("BuildSubsequentReviewPrompt() missing %q\nGot: %s", want, got)
				}
			}
		})
	}
}

func TestGetSubsequentReviewSystemPrompt(t *testing.T) {
	tests := []struct {
		name         string
		claudeMD     string
		instructions string
		wantContains []string
	}{
		{
			name:         "base system prompt",
			claudeMD:     "",
			instructions: "",
			wantContains: []string{
				"SUBSEQUENT review",
				"blocker",
				"suggestion",
				"nitpick",
				"Resolved comments",
			},
		},
		{
			name:         "with claudeMD",
			claudeMD:     "Project uses Go 1.21",
			instructions: "",
			wantContains: []string{
				"Project Context",
				"Project uses Go 1.21",
			},
		},
		{
			name:         "with instructions",
			claudeMD:     "",
			instructions: "Focus on security",
			wantContains: []string{
				"Repository-Specific Instructions",
				"Focus on security",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSubsequentReviewSystemPrompt(tt.claudeMD, tt.instructions)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("GetSubsequentReviewSystemPrompt() missing %q", want)
				}
			}
		})
	}
}

func TestFormatExistingComments(t *testing.T) {
	tests := []struct {
		name     string
		comments []ExistingComment
		want     string
	}{
		{
			name:     "empty comments",
			comments: []ExistingComment{},
			want:     "",
		},
		{
			name: "truncates long bodies",
			comments: []ExistingComment{
				{
					Path:       "main.go",
					Line:       1,
					Body:       strings.Repeat("a", 250),
					IsResolved: false,
					Author:     "user",
				},
			},
			want: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExistingComments(tt.comments)
			if tt.want == "" && got != "" {
				t.Errorf("formatExistingComments() = %v, want empty", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("formatExistingComments() missing %q\nGot: %s", tt.want, got)
			}
		})
	}
}
