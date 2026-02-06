package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	secret := "test-secret"
	handler := NewWebhookHandler(secret)

	// Test payload
	payload := []byte(`{"action": "opened"}`)

	// Generate valid signature using HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Generate invalid signature (wrong content)
	wrongMac := hmac.New(sha256.New, []byte(secret))
	wrongMac.Write([]byte(`{"action": "closed"}`))
	wrongSignature := "sha256=" + hex.EncodeToString(wrongMac.Sum(nil))

	tests := []struct {
		name      string
		signature string
		wantErr   error
	}{
		{
			name:      "missing signature",
			signature: "",
			wantErr:   ErrMissingSignature,
		},
		{
			name:      "invalid format",
			signature: "invalid",
			wantErr:   ErrInvalidSignature,
		},
		{
			name:      "wrong algorithm",
			signature: "sha1=abc123",
			wantErr:   ErrInvalidSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.VerifySignature(payload, tt.signature)
			if err != tt.wantErr {
				t.Errorf("VerifySignature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// Test with invalid hex in signature
	t.Run("invalid hex", func(t *testing.T) {
		err := handler.VerifySignature(payload, "sha256=zzzz")
		if err == nil {
			t.Error("VerifySignature() expected error for invalid hex")
		}
	})

	// Test valid signature
	t.Run("valid signature", func(t *testing.T) {
		err := handler.VerifySignature(payload, validSignature)
		if err != nil {
			t.Errorf("VerifySignature() unexpected error = %v", err)
		}
	})

	// Test signature mismatch
	t.Run("signature mismatch", func(t *testing.T) {
		err := handler.VerifySignature(payload, wrongSignature)
		if err != ErrInvalidSignature {
			t.Errorf("VerifySignature() expected ErrInvalidSignature, got %v", err)
		}
	})
}

func TestShouldProcess(t *testing.T) {
	handler := NewWebhookHandler("secret")

	tests := []struct {
		name      string
		eventType string
		action    string
		want      bool
	}{
		{
			name:      "pull_request opened",
			eventType: "pull_request",
			action:    "opened",
			want:      true,
		},
		{
			name:      "pull_request synchronize",
			eventType: "pull_request",
			action:    "synchronize",
			want:      true,
		},
		{
			name:      "pull_request reopened",
			eventType: "pull_request",
			action:    "reopened",
			want:      true,
		},
		{
			name:      "pull_request closed",
			eventType: "pull_request",
			action:    "closed",
			want:      false,
		},
		{
			name:      "push event",
			eventType: "push",
			action:    "",
			want:      false,
		},
		{
			name:      "issue_comment event",
			eventType: "issue_comment",
			action:    "created",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &WebhookEvent{Action: tt.action}
			if got := handler.ShouldProcess(tt.eventType, event); got != tt.want {
				t.Errorf("ShouldProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePullRequestEvent(t *testing.T) {
	handler := NewWebhookHandler("secret")

	t.Run("valid payload", func(t *testing.T) {
		payload := []byte(`{
			"action": "opened",
			"number": 42,
			"pull_request": {
				"id": 123,
				"number": 42,
				"title": "Test PR",
				"head": {"sha": "abc123", "ref": "feature"},
				"base": {"sha": "def456", "ref": "main"}
			},
			"repository": {
				"id": 789,
				"name": "test-repo",
				"full_name": "owner/test-repo",
				"owner": {"login": "owner"}
			},
			"installation": {"id": 999}
		}`)

		event, err := handler.ParsePullRequestEvent(payload)
		if err != nil {
			t.Fatalf("ParsePullRequestEvent() error = %v", err)
		}

		if event.Action != "opened" {
			t.Errorf("Action = %v, want opened", event.Action)
		}
		if event.Number != 42 {
			t.Errorf("Number = %v, want 42", event.Number)
		}
		if event.PullRequest.Title != "Test PR" {
			t.Errorf("Title = %v, want Test PR", event.PullRequest.Title)
		}
		if event.Installation.ID != 999 {
			t.Errorf("Installation.ID = %v, want 999", event.Installation.ID)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := handler.ParsePullRequestEvent([]byte(`{invalid`))
		if err == nil {
			t.Error("ParsePullRequestEvent() expected error for invalid JSON")
		}
	})

	t.Run("missing pull_request", func(t *testing.T) {
		_, err := handler.ParsePullRequestEvent([]byte(`{"action": "opened"}`))
		if err == nil {
			t.Error("ParsePullRequestEvent() expected error for missing pull_request")
		}
	})
}

func TestContainsMention(t *testing.T) {
	tests := []struct {
		text     string
		username string
		want     bool
	}{
		{"@shipitai can you explain?", "shipitai", true},
		{"Hey @shipitai what about this?", "shipitai", true},
		{"@SHIPITAI please clarify", "shipitai", true},
		{"@ShipItAI mixed case", "shipitai", true},
		{"no mention here", "shipitai", false},
		{"shipitai without @", "shipitai", false},
		{"email@shipitai.com", "shipitai", false}, // email addresses should not trigger
		{"@other-bot please help", "shipitai", false},
		{"@shipitai", "shipitai", true},                       // mention at end of string
		{"@shipitai, thanks!", "shipitai", true},              // followed by comma
		{"(@shipitai)", "shipitai", true},                     // in parentheses (preceded by non-space)
		{"Check with @shipitai.", "shipitai", true},           // followed by period at end
		{"contact support@shipitai.dev for help", "shipitai", false}, // email with different domain
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := ContainsMention(tt.text, tt.username); got != tt.want {
				t.Errorf("ContainsMention(%q, %q) = %v, want %v", tt.text, tt.username, got, tt.want)
			}
		})
	}
}

func TestExtractMentionContext(t *testing.T) {
	tests := []struct {
		text     string
		username string
		want     string
	}{
		{"@shipitai can you explain?", "shipitai", "can you explain?"},
		{"Hey @shipitai what about this?", "shipitai", "Hey  what about this?"},
		{"@SHIPITAI please clarify", "shipitai", "please clarify"},
		{"no mention here", "shipitai", "no mention here"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := ExtractMentionContext(tt.text, tt.username); got != tt.want {
				t.Errorf("ExtractMentionContext(%q, %q) = %q, want %q", tt.text, tt.username, got, tt.want)
			}
		})
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		text     string
		botName  string
		want     string
	}{
		{"@shipitai review", "shipitai", "review"},
		{"@shipitai please review this", "shipitai", "review"},
		{"@SHIPITAI review", "shipitai", "review"},
		{"Hey @shipitai can you review?", "shipitai", "review"},
		{"@shipitai", "shipitai", ""},
		{"@shipitai help me", "shipitai", ""},
		{"no mention here", "shipitai", ""},
		{"@other-bot review", "shipitai", ""},
		{"@shipitai REVIEW please", "shipitai", "review"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := ExtractCommand(tt.text, tt.botName); got != tt.want {
				t.Errorf("ExtractCommand(%q, %q) = %q, want %q", tt.text, tt.botName, got, tt.want)
			}
		})
	}
}

func TestParseIssueCommentEvent(t *testing.T) {
	handler := NewWebhookHandler("secret")

	t.Run("valid payload", func(t *testing.T) {
		payload := []byte(`{
			"action": "created",
			"issue": {
				"number": 42,
				"title": "Test PR",
				"pull_request": {"url": "https://api.github.com/repos/owner/repo/pulls/42"}
			},
			"comment": {
				"id": 123,
				"body": "@shipitai review",
				"user": {"login": "contributor"}
			},
			"repository": {
				"id": 789,
				"name": "test-repo",
				"owner": {"login": "owner"}
			},
			"installation": {"id": 999},
			"sender": {"login": "contributor"}
		}`)

		event, err := handler.ParseIssueCommentEvent(payload)
		if err != nil {
			t.Fatalf("ParseIssueCommentEvent() error = %v", err)
		}

		if event.Action != "created" {
			t.Errorf("Action = %v, want created", event.Action)
		}
		if event.Issue.Number != 42 {
			t.Errorf("Issue.Number = %v, want 42", event.Issue.Number)
		}
		if event.Comment.Body != "@shipitai review" {
			t.Errorf("Comment.Body = %v, want @shipitai review", event.Comment.Body)
		}
	})

	t.Run("missing comment", func(t *testing.T) {
		payload := []byte(`{"action": "created", "issue": {"number": 42}}`)
		_, err := handler.ParseIssueCommentEvent(payload)
		if err == nil {
			t.Error("ParseIssueCommentEvent() expected error for missing comment")
		}
	})

	t.Run("missing issue", func(t *testing.T) {
		payload := []byte(`{"action": "created", "comment": {"body": "test"}}`)
		_, err := handler.ParseIssueCommentEvent(payload)
		if err == nil {
			t.Error("ParseIssueCommentEvent() expected error for missing issue")
		}
	})
}

func TestShouldProcessIssueComment(t *testing.T) {
	handler := NewWebhookHandler("secret")

	tests := []struct {
		name    string
		event   *IssueCommentEvent
		botName string
		want    bool
	}{
		{
			name: "valid PR comment with mention",
			event: &IssueCommentEvent{
				Action: "created",
				Issue: &Issue{
					Number:      42,
					PullRequest: &IssuePRLink{URL: "https://api.github.com/repos/owner/repo/pulls/42"},
				},
				Comment: &IssueComment{Body: "@shipitai review"},
			},
			botName: "shipitai",
			want:    true,
		},
		{
			name: "not created action",
			event: &IssueCommentEvent{
				Action: "edited",
				Issue: &Issue{
					Number:      42,
					PullRequest: &IssuePRLink{URL: "https://api.github.com/repos/owner/repo/pulls/42"},
				},
				Comment: &IssueComment{Body: "@shipitai review"},
			},
			botName: "shipitai",
			want:    false,
		},
		{
			name: "issue comment (not PR)",
			event: &IssueCommentEvent{
				Action:  "created",
				Issue:   &Issue{Number: 42}, // No PullRequest link
				Comment: &IssueComment{Body: "@shipitai review"},
			},
			botName: "shipitai",
			want:    false,
		},
		{
			name: "no mention",
			event: &IssueCommentEvent{
				Action: "created",
				Issue: &Issue{
					Number:      42,
					PullRequest: &IssuePRLink{URL: "https://api.github.com/repos/owner/repo/pulls/42"},
				},
				Comment: &IssueComment{Body: "looks good to me"},
			},
			botName: "shipitai",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handler.ShouldProcessIssueComment(tt.event, tt.botName); got != tt.want {
				t.Errorf("ShouldProcessIssueComment() = %v, want %v", got, tt.want)
			}
		})
	}
}
