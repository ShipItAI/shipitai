package review

import (
	"strconv"
	"strings"
	"testing"
)

func TestAnnotateDiffWithLineNumbers(t *testing.T) {
	tests := []struct {
		name string
		diff string
		want string
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
			want: `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -10,3 +10,5 @@ func main() {
   10 |  	fmt.Println("existing")
   11 | +	fmt.Println("new line 1")
   12 | +	fmt.Println("new line 2")
   13 |  	fmt.Println("also existing")
   14 |  }`,
		},
		{
			name: "deletion has no line number",
			diff: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -10,4 +10,2 @@ func main() {
 	fmt.Println("keep")
-	fmt.Println("remove 1")
-	fmt.Println("remove 2")
 	fmt.Println("also keep")`,
			want: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -10,4 +10,2 @@ func main() {
   10 |  	fmt.Println("keep")
      | -	fmt.Println("remove 1")
      | -	fmt.Println("remove 2")
   11 |  	fmt.Println("also keep")`,
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
			want: "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -5,3 +5,4 @@ package main\n    5 |  import \"fmt\"\n    6 | +import \"os\"\n    7 | \n    8 |  func main() {\n@@ -20,2 +21,3 @@ func main() {\n   21 |  \tfmt.Println(\"end\")\n   22 | +\tos.Exit(0)\n   23 |  }",
		},
		{
			name: "new file starts at line 1",
			diff: `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abc123
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package new
+
+func New() {}`,
			want: `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abc123
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
    1 | +package new
    2 | +
    3 | +func New() {}`,
		},
		{
			name: "deleted file has no line numbers",
			diff: `diff --git a/old.go b/old.go
deleted file mode 100644
index abc123..0000000
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package old
-
-func Old() {}`,
			want: `diff --git a/old.go b/old.go
deleted file mode 100644
index abc123..0000000
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
      | -package old
      | -
      | -func Old() {}`,
		},
		{
			name: "no newline at end of file marker passes through",
			diff: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
-var x = 1
+var x = 2
\ No newline at end of file`,
			want: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
    1 |  package main
      | -var x = 1
    2 | +var x = 2
\ No newline at end of file`,
		},
		{
			name: "empty diff",
			diff: "",
			want: "",
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
			want: "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1,2 +1,3 @@\n    1 |  package foo\n    2 | +var x = 1\n    3 | \ndiff --git a/bar.go b/bar.go\n--- a/bar.go\n+++ b/bar.go\n@@ -10,2 +10,3 @@\n   10 |  func bar() {\n   11 | +\treturn\n   12 |  }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnnotateDiffWithLineNumbers(tt.diff)
			if got != tt.want {
				t.Errorf("AnnotateDiffWithLineNumbers() mismatch\ngot:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestAnnotateDiffLineNumbersAreCorrect(t *testing.T) {
	// This test verifies that the annotated line numbers match what ParseDiffLines computes,
	// ensuring consistency between the two functions.
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -5,6 +5,8 @@ package main
 import "fmt"
+import "os"

 func main() {
-	fmt.Println("old")
+	fmt.Println("new")
+	os.Exit(0)
 }`

	diffLines := ParseDiffLines(diff)
	annotated := AnnotateDiffWithLineNumbers(diff)

	// Extract line numbers from annotation
	for _, line := range strings.Split(annotated, "\n") {
		if len(line) < 8 || line[5] != ' ' || line[6] != '|' {
			continue // Skip non-annotated lines (headers, etc.)
		}
		numStr := strings.TrimSpace(line[:5])
		if numStr == "" {
			continue // Deleted line, no number
		}
		lineNum, err := strconv.Atoi(numStr)
		if err != nil {
			t.Errorf("failed to parse line number from %q: %v", line[:5], err)
			continue
		}
		if !diffLines.IsValidCommentLine("main.go", lineNum) {
			t.Errorf("annotated line %d is not valid according to ParseDiffLines", lineNum)
		}
	}
}

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
		{
			name:     "preamble text before json code block",
			response: "Looking at the diff carefully, I need to identify new issues.\n\n```json\n{\"summary\": \"LGTM\", \"comments\": [], \"approval\": \"approve\"}\n```",
			wantErr:  false,
			validate: func(r *ClaudeResponse) error {
				if r.Summary != "LGTM" {
					t.Errorf("Summary = %v, want LGTM", r.Summary)
				}
				if r.Approval != "approve" {
					t.Errorf("Approval = %v, want approve", r.Approval)
				}
				return nil
			},
		},
		{
			name:     "preamble text before bare JSON",
			response: "Let me analyze this PR.\n\n{\"summary\": \"Issues found\", \"comments\": [{\"path\": \"main.go\", \"line\": 10, \"body\": \"Bug here\", \"severity\": \"critical\"}], \"approval\": \"request_changes\"}",
			wantErr:  false,
			validate: func(r *ClaudeResponse) error {
				if r.Summary != "Issues found" {
					t.Errorf("Summary = %v, want Issues found", r.Summary)
				}
				if len(r.Comments) != 1 {
					t.Errorf("Comments length = %v, want 1", len(r.Comments))
				}
				if r.Approval != "request_changes" {
					t.Errorf("Approval = %v, want request_changes", r.Approval)
				}
				return nil
			},
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
				if err := tt.validate(result); err != nil {
					t.Errorf("validate() failed: %v", err)
				}
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
		{
			name:  "preamble text before json code block",
			input: "Looking at the diff carefully, I need to identify new issues.\n\n```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "preamble text before plain code block",
			input: "Let me analyze this.\n\n```\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "preamble text before bare JSON",
			input: "Looking at the diff carefully.\n\n{\"key\": \"value\"}",
			want:  `{"key": "value"}`,
		},
		{
			name:  "preamble with bare JSON and trailing text",
			input: "Some thinking.\n\n{\"key\": \"value\"}\n\nDone.",
			want:  `{"key": "value"}`,
		},
		{
			name:  "bare suggestion fragment with no JSON returns as-is",
			input: "suggestion\nfixed code here\n",
			want:  "suggestion\nfixed code here",
		},
		{
			name:  "JSON embedded after suggestion text",
			input: "suggestion\nfixed code here\n\n{\"summary\": \"Test\", \"comments\": [], \"approval\": \"approve\"}",
			want:  `{"summary": "Test", "comments": [], "approval": "approve"}`,
		},
		{
			name:  "prefilled brace idempotent",
			input: `{"summary": "Test", "comments": [], "approval": "approve"}`,
			want:  `{"summary": "Test", "comments": [], "approval": "approve"}`,
		},
		{
			name:  "JSON with suggestion blocks in comment body not confused by backticks",
			input: "{\"summary\": \"Test\", \"comments\": [{\"path\": \"main.go\", \"line\": 10, \"body\": \"Fix:\\n```suggestion\\nfixed\\n```\", \"severity\": \"medium\"}], \"approval\": \"comment\"}",
			want:  "{\"summary\": \"Test\", \"comments\": [{\"path\": \"main.go\", \"line\": 10, \"body\": \"Fix:\\n```suggestion\\nfixed\\n```\", \"severity\": \"medium\"}], \"approval\": \"comment\"}",
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
					{"path": "main.go", "line": 42, "body": "Critical bug", "severity": "critical"},
					{"path": "util.go", "line": 10, "body": "Nice to have", "severity": "medium"},
					{"path": "test.go", "line": 5, "body": "Minor style", "severity": "low"}
				],
				"approval": "request_changes"
			}`,
			wantErr: false,
			validate: func(r *ClaudeResponse) error {
				if len(r.Comments) != 3 {
					t.Errorf("Comments length = %v, want 3", len(r.Comments))
				}
				if r.Comments[0].Severity != "critical" {
					t.Errorf("Comment 0 severity = %v, want critical", r.Comments[0].Severity)
				}
				if r.Comments[1].Severity != "medium" {
					t.Errorf("Comment 1 severity = %v, want medium", r.Comments[1].Severity)
				}
				if r.Comments[2].Severity != "low" {
					t.Errorf("Comment 2 severity = %v, want low", r.Comments[2].Severity)
				}
				return nil
			},
		},
		{
			name: "missing severity defaults to medium",
			response: `{
				"summary": "Test",
				"comments": [
					{"path": "main.go", "line": 42, "body": "Some comment"}
				],
				"approval": "comment"
			}`,
			wantErr: false,
			validate: func(r *ClaudeResponse) error {
				if r.Comments[0].Severity != "medium" {
					t.Errorf("Comment severity = %v, want medium (default)", r.Comments[0].Severity)
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
				if err := tt.validate(result); err != nil {
					t.Errorf("validate() failed: %v", err)
				}
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
			name: "only critical -> request_changes",
			comments: []ClaudeComment{
				{Severity: "critical"},
			},
			want: "request_changes",
		},
		{
			name: "only high -> request_changes",
			comments: []ClaudeComment{
				{Severity: "high"},
			},
			want: "request_changes",
		},
		{
			name: "only medium -> comment",
			comments: []ClaudeComment{
				{Severity: "medium"},
			},
			want: "comment",
		},
		{
			name: "only low -> approve",
			comments: []ClaudeComment{
				{Severity: "low"},
			},
			want: "approve",
		},
		{
			name: "mixed with critical -> request_changes",
			comments: []ClaudeComment{
				{Severity: "medium"},
				{Severity: "critical"},
				{Severity: "low"},
			},
			want: "request_changes",
		},
		{
			name: "medium and low -> comment",
			comments: []ClaudeComment{
				{Severity: "medium"},
				{Severity: "low"},
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
			name:     "critical adds bold badge",
			body:     "Critical issue here",
			severity: "critical",
			want:     "**[critical]** Critical issue here",
		},
		{
			name:     "high adds bold badge",
			body:     "Important issue here",
			severity: "high",
			want:     "**[high]** Important issue here",
		},
		{
			name:     "low adds italic badge",
			body:     "Minor style issue",
			severity: "low",
			want:     "*[low]* Minor style issue",
		},
		{
			name:     "medium has no badge",
			body:     "Consider this change",
			severity: "medium",
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
