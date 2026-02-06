package review

import (
	"testing"
)

func TestParseDiffLines(t *testing.T) {
	tests := []struct {
		name     string
		diff     string
		expected map[string][]int // file -> expected valid lines
	}{
		{
			name: "simple addition",
			diff: `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -10,3 +10,5 @@ func main() {
 	fmt.Println("existing")
+	fmt.Println("new line 1")
+	fmt.Println("new line 2")
 	fmt.Println("also existing")
 }`,
			expected: map[string][]int{
				"main.go": {10, 11, 12, 13, 14}, // context + added + context
			},
		},
		{
			name: "deletion only",
			diff: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -10,4 +10,2 @@ func main() {
 	fmt.Println("keep")
-	fmt.Println("remove 1")
-	fmt.Println("remove 2")
 	fmt.Println("also keep")`,
			expected: map[string][]int{
				"main.go": {10, 11}, // only context lines remain
			},
		},
		{
			name: "multiple hunks",
			diff: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -5,3 +5,4 @@ package main
 import "fmt"
+import "os"

 func main() {
@@ -20,2 +21,3 @@ func main() {
 	fmt.Println("end")
+	os.Exit(0)
 }`,
			expected: map[string][]int{
				"main.go": {5, 6, 7, 8, 21, 22, 23}, // lines from both hunks
			},
		},
		{
			name: "multiple files",
			diff: `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,2 +1,3 @@
 package foo
+var x = 1

diff --git a/bar.go b/bar.go
--- a/bar.go
+++ b/bar.go
@@ -10,2 +10,3 @@
 func bar() {
+	return
 }`,
			expected: map[string][]int{
				"foo.go": {1, 2, 3},
				"bar.go": {10, 11, 12},
			},
		},
		{
			name: "new file",
			diff: `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abc123
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package new
+
+func New() {}`,
			expected: map[string][]int{
				"new.go": {1, 2, 3},
			},
		},
		{
			name: "deleted file",
			diff: `diff --git a/old.go b/old.go
deleted file mode 100644
index abc123..0000000
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package old
-
-func Old() {}`,
			expected: map[string][]int{}, // no valid lines in deleted file
		},
		{
			name:     "empty diff",
			diff:     "",
			expected: map[string][]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseDiffLines(tt.diff)

			// Check each expected file
			for file, expectedLines := range tt.expected {
				for _, line := range expectedLines {
					if !result.IsValidCommentLine(file, line) {
						t.Errorf("expected line %d in %s to be valid", line, file)
					}
				}
			}

			// Check that invalid lines are not marked valid
			for file := range tt.expected {
				// Test a line that shouldn't be valid (very high line number)
				if result.IsValidCommentLine(file, 9999) {
					t.Errorf("line 9999 in %s should not be valid", file)
				}
			}

			// Check non-existent file
			if result.IsValidCommentLine("nonexistent.go", 1) {
				t.Error("nonexistent file should not have valid lines")
			}
		})
	}
}

func TestFilterValidComments(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -10,3 +10,4 @@
 	existing()
+	newLine()
 	alsoExisting()
 }`
	diffLines := ParseDiffLines(diff)

	tests := []struct {
		name             string
		comments         []ClaudeComment
		wantValid        int
		wantFiltered     int
	}{
		{
			name: "all valid",
			comments: []ClaudeComment{
				{Path: "main.go", Line: 10, Body: "comment on context line"},
				{Path: "main.go", Line: 11, Body: "comment on added line"},
				{Path: "main.go", Line: 12, Body: "another context line"},
			},
			wantValid:    3,
			wantFiltered: 0,
		},
		{
			name: "some invalid",
			comments: []ClaudeComment{
				{Path: "main.go", Line: 11, Body: "valid"},
				{Path: "main.go", Line: 100, Body: "invalid - line not in diff"},
				{Path: "main.go", Line: 12, Body: "valid"},
			},
			wantValid:    2,
			wantFiltered: 1,
		},
		{
			name: "invalid file path",
			comments: []ClaudeComment{
				{Path: "other.go", Line: 10, Body: "wrong file"},
			},
			wantValid:    0,
			wantFiltered: 1,
		},
		{
			name: "all invalid",
			comments: []ClaudeComment{
				{Path: "main.go", Line: 1, Body: "line 1 not in hunk"},
				{Path: "main.go", Line: 500, Body: "way out of range"},
				{Path: "wrong.go", Line: 10, Body: "wrong file"},
			},
			wantValid:    0,
			wantFiltered: 3,
		},
		{
			name:         "empty comments",
			comments:     []ClaudeComment{},
			wantValid:    0,
			wantFiltered: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, filtered := FilterValidComments(tt.comments, diffLines, nil)

			if len(valid) != tt.wantValid {
				t.Errorf("got %d valid comments, want %d", len(valid), tt.wantValid)
			}
			if filtered != tt.wantFiltered {
				t.Errorf("got %d filtered comments, want %d", filtered, tt.wantFiltered)
			}
		})
	}
}

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
		validate func(*ClaudeResponse) error
	}{
		{
			name: "valid response",
			response: `{
				"summary": "Good PR with minor issues",
				"comments": [
					{"path": "main.go", "line": 42, "body": "Consider error handling here"}
				],
				"approval": "comment"
			}`,
			wantErr: false,
			validate: func(r *ClaudeResponse) error {
				if r.Summary != "Good PR with minor issues" {
					t.Errorf("Summary = %v", r.Summary)
				}
				if len(r.Comments) != 1 {
					t.Errorf("Comments length = %v, want 1", len(r.Comments))
				}
				if r.Approval != "comment" {
					t.Errorf("Approval = %v, want comment", r.Approval)
				}
				return nil
			},
		},
		{
			name: "with markdown code block",
			response: "```json\n{\"summary\": \"Test\", \"comments\": [], \"approval\": \"approve\"}\n```",
			wantErr: false,
			validate: func(r *ClaudeResponse) error {
				if r.Approval != "approve" {
					t.Errorf("Approval = %v, want approve", r.Approval)
				}
				return nil
			},
		},
		{
			name: "empty comments",
			response: `{"summary": "LGTM", "comments": [], "approval": "approve"}`,
			wantErr: false,
		},
		{
			name:     "invalid JSON",
			response: `{invalid`,
			wantErr:  true,
		},
		{
			name:     "invalid approval value",
			response: `{"summary": "Test", "comments": [], "approval": "invalid"}`,
			wantErr:  true,
		},
		{
			name:     "comment with empty path",
			response: `{"summary": "Test", "comments": [{"path": "", "line": 1, "body": "test"}], "approval": "comment"}`,
			wantErr:  true,
		},
		{
			name:     "comment with zero line",
			response: `{"summary": "Test", "comments": [{"path": "main.go", "line": 0, "body": "test"}], "approval": "comment"}`,
			wantErr:  true,
		},
		{
			name:     "comment with empty body",
			response: `{"summary": "Test", "comments": [{"path": "main.go", "line": 1, "body": ""}], "approval": "comment"}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseResponse(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(result)
			}
		})
	}
}

func TestCleanResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain JSON",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "with json code block",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "with plain code block",
			input: "```\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "with whitespace",
			input: "  \n{\"key\": \"value\"}\n  ",
			want:  `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanResponse(tt.input); got != tt.want {
				t.Errorf("cleanResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseResponseWithSeverity(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
		validate func(*ClaudeResponse) error
	}{
		{
			name: "valid response with severity",
			response: `{
				"summary": "Issues found",
				"comments": [
					{"path": "main.go", "line": 42, "body": "Critical bug", "severity": "blocker"},
					{"path": "util.go", "line": 10, "body": "Nice to have", "severity": "suggestion"},
					{"path": "test.go", "line": 5, "body": "Minor style", "severity": "nitpick"}
				],
				"approval": "request_changes"
			}`,
			wantErr: false,
			validate: func(r *ClaudeResponse) error {
				if len(r.Comments) != 3 {
					t.Errorf("Comments length = %v, want 3", len(r.Comments))
				}
				if r.Comments[0].Severity != "blocker" {
					t.Errorf("Comment 0 severity = %v, want blocker", r.Comments[0].Severity)
				}
				if r.Comments[1].Severity != "suggestion" {
					t.Errorf("Comment 1 severity = %v, want suggestion", r.Comments[1].Severity)
				}
				if r.Comments[2].Severity != "nitpick" {
					t.Errorf("Comment 2 severity = %v, want nitpick", r.Comments[2].Severity)
				}
				return nil
			},
		},
		{
			name: "missing severity defaults to suggestion",
			response: `{
				"summary": "Test",
				"comments": [
					{"path": "main.go", "line": 42, "body": "Some comment"}
				],
				"approval": "comment"
			}`,
			wantErr: false,
			validate: func(r *ClaudeResponse) error {
				if r.Comments[0].Severity != "suggestion" {
					t.Errorf("Comment severity = %v, want suggestion (default)", r.Comments[0].Severity)
				}
				return nil
			},
		},
		{
			name: "invalid severity value",
			response: `{
				"summary": "Test",
				"comments": [
					{"path": "main.go", "line": 42, "body": "Comment", "severity": "invalid"}
				],
				"approval": "comment"
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseResponse(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(result)
			}
		})
	}
}

func TestDetermineApprovalFromSeverity(t *testing.T) {
	tests := []struct {
		name     string
		comments []ClaudeComment
		want     string
	}{
		{
			name:     "no comments -> approve",
			comments: []ClaudeComment{},
			want:     "approve",
		},
		{
			name: "only blockers -> request_changes",
			comments: []ClaudeComment{
				{Severity: "blocker"},
			},
			want: "request_changes",
		},
		{
			name: "only suggestions -> comment",
			comments: []ClaudeComment{
				{Severity: "suggestion"},
			},
			want: "comment",
		},
		{
			name: "only nitpicks -> comment",
			comments: []ClaudeComment{
				{Severity: "nitpick"},
			},
			want: "comment",
		},
		{
			name: "mixed with blocker -> request_changes",
			comments: []ClaudeComment{
				{Severity: "suggestion"},
				{Severity: "blocker"},
				{Severity: "nitpick"},
			},
			want: "request_changes",
		},
		{
			name: "suggestions and nitpicks -> comment",
			comments: []ClaudeComment{
				{Severity: "suggestion"},
				{Severity: "nitpick"},
			},
			want: "comment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineApprovalFromSeverity(tt.comments)
			if got != tt.want {
				t.Errorf("DetermineApprovalFromSeverity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatCommentWithSeverity(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		severity string
		want     string
	}{
		{
			name:     "blocker adds bold badge",
			body:     "Critical issue here",
			severity: "blocker",
			want:     "**[BLOCKER]** Critical issue here",
		},
		{
			name:     "nitpick adds italic badge",
			body:     "Minor style issue",
			severity: "nitpick",
			want:     "*[nitpick]* Minor style issue",
		},
		{
			name:     "suggestion has no badge",
			body:     "Consider this change",
			severity: "suggestion",
			want:     "Consider this change",
		},
		{
			name:     "empty severity has no badge",
			body:     "Some comment",
			severity: "",
			want:     "Some comment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCommentWithSeverity(tt.body, tt.severity)
			if got != tt.want {
				t.Errorf("FormatCommentWithSeverity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToGitHubReview(t *testing.T) {
	tests := []struct {
		name       string
		response   *ClaudeResponse
		commitSHA  string
		wantEvent  string
		wantComments int
	}{
		{
			name: "approve",
			response: &ClaudeResponse{
				Summary:  "LGTM",
				Comments: nil,
				Approval: "approve",
			},
			commitSHA:    "abc123",
			wantEvent:    "APPROVE",
			wantComments: 0,
		},
		{
			name: "request_changes",
			response: &ClaudeResponse{
				Summary: "Issues found",
				Comments: []ClaudeComment{
					{Path: "main.go", Line: 10, Body: "Bug here"},
				},
				Approval: "request_changes",
			},
			commitSHA:    "abc123",
			wantEvent:    "REQUEST_CHANGES",
			wantComments: 1,
		},
		{
			name: "comment",
			response: &ClaudeResponse{
				Summary: "Some suggestions",
				Comments: []ClaudeComment{
					{Path: "main.go", Line: 10, Body: "Consider this"},
					{Path: "util.go", Line: 20, Body: "Nice approach"},
				},
				Approval: "comment",
			},
			commitSHA:    "abc123",
			wantEvent:    "COMMENT",
			wantComments: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			review, err := ToGitHubReview(tt.response, tt.commitSHA)
			if err != nil {
				t.Fatalf("ToGitHubReview() error = %v", err)
			}

			if review.Event != tt.wantEvent {
				t.Errorf("Event = %v, want %v", review.Event, tt.wantEvent)
			}
			if len(review.Comments) != tt.wantComments {
				t.Errorf("Comments length = %v, want %v", len(review.Comments), tt.wantComments)
			}
			if review.CommitID != tt.commitSHA {
				t.Errorf("CommitID = %v, want %v", review.CommitID, tt.commitSHA)
			}
		})
	}
}
