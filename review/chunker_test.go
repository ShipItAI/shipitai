package review

import (
	"strings"
	"testing"
)

func TestSplitDiffByFile(t *testing.T) {
	tests := []struct {
		name     string
		diff     string
		wantLen  int
		wantPath string // first file path
	}{
		{
			name:    "empty diff",
			diff:    "",
			wantLen: 0,
		},
		{
			name: "single file",
			diff: `diff --git a/foo.go b/foo.go
index 1234567..abcdefg 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package foo
+// added line`,
			wantLen:  1,
			wantPath: "foo.go",
		},
		{
			name: "multiple files",
			diff: `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1 +1 @@
-old
+new
diff --git a/bar.go b/bar.go
--- a/bar.go
+++ b/bar.go
@@ -1 +1 @@
-old
+new
diff --git a/baz.go b/baz.go
--- a/baz.go
+++ b/baz.go
@@ -1 +1 @@
-old
+new`,
			wantLen:  3,
			wantPath: "foo.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := SplitDiffByFile(tt.diff)
			if len(files) != tt.wantLen {
				t.Errorf("SplitDiffByFile() got %d files, want %d", len(files), tt.wantLen)
			}
			if tt.wantLen > 0 && files[0].Path != tt.wantPath {
				t.Errorf("SplitDiffByFile() first file path = %q, want %q", files[0].Path, tt.wantPath)
			}
		})
	}
}

func TestSplitDiffByFile_ContentPreserved(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1 +1 @@
-old line
+new line`

	files := SplitDiffByFile(diff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	if !strings.Contains(files[0].Content, "+new line") {
		t.Error("content should contain +new line")
	}
	if !strings.Contains(files[0].Content, "-old line") {
		t.Error("content should contain -old line")
	}
}

func TestChunkDiff(t *testing.T) {
	// Create a diff with 3 files, each 100 bytes
	file1 := "diff --git a/f1.go b/f1.go\n" + strings.Repeat("a", 70) + "\n"
	file2 := "diff --git a/f2.go b/f2.go\n" + strings.Repeat("b", 70) + "\n"
	file3 := "diff --git a/f3.go b/f3.go\n" + strings.Repeat("c", 70) + "\n"
	diff := file1 + file2 + file3

	// With a 200 byte limit, should get 2 chunks
	chunks := ChunkDiff(diff, 200)

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}

	// Verify indices are set correctly
	for i, chunk := range chunks {
		if chunk.Index != i {
			t.Errorf("chunk %d: Index = %d, want %d", i, chunk.Index, i)
		}
		if chunk.Total != 2 {
			t.Errorf("chunk %d: Total = %d, want 2", i, chunk.Total)
		}
	}
}

func TestChunkDiff_OversizedFile(t *testing.T) {
	// Single file larger than max chunk size
	oversized := "diff --git a/big.go b/big.go\n" + strings.Repeat("x", 1000) + "\n"

	chunks := ChunkDiff(oversized, 500)

	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for oversized file, got %d", len(chunks))
	}

	if len(chunks[0].Files) != 1 {
		t.Errorf("expected 1 file in oversized chunk, got %d", len(chunks[0].Files))
	}
}

func TestChunkDiff_Empty(t *testing.T) {
	chunks := ChunkDiff("", 1000)
	if chunks != nil {
		t.Errorf("expected nil chunks for empty diff, got %v", chunks)
	}
}

func TestChunkToDiff(t *testing.T) {
	chunk := &Chunk{
		Files: []FileDiff{
			{Path: "foo.go", Content: "diff --git a/foo.go b/foo.go\n+line1"},
			{Path: "bar.go", Content: "diff --git a/bar.go b/bar.go\n+line2"},
		},
	}

	diff := ChunkToDiff(chunk)

	if !strings.Contains(diff, "foo.go") {
		t.Error("diff should contain foo.go")
	}
	if !strings.Contains(diff, "bar.go") {
		t.Error("diff should contain bar.go")
	}
}

func TestMergeChunkResponses(t *testing.T) {
	tests := []struct {
		name         string
		results      []*ChunkResult
		wantApproval string
		wantComments int
	}{
		{
			name:         "empty results",
			results:      []*ChunkResult{},
			wantApproval: "comment",
			wantComments: 0,
		},
		{
			name: "single chunk approve",
			results: []*ChunkResult{
				{
					Index: 0,
					Response: &ClaudeResponse{
						Summary:  "Looks good",
						Approval: "approve",
						Comments: []ClaudeComment{},
					},
				},
			},
			wantApproval: "approve",
			wantComments: 0,
		},
		{
			name: "multiple chunks with comments",
			results: []*ChunkResult{
				{
					Index: 0,
					Response: &ClaudeResponse{
						Summary:  "Part 1 ok",
						Approval: "comment",
						Comments: []ClaudeComment{
							{Path: "a.go", Line: 1, Body: "comment 1"},
						},
					},
				},
				{
					Index: 1,
					Response: &ClaudeResponse{
						Summary:  "Part 2 ok",
						Approval: "approve",
						Comments: []ClaudeComment{
							{Path: "b.go", Line: 2, Body: "comment 2"},
						},
					},
				},
			},
			wantApproval: "comment", // strictest wins
			wantComments: 2,
		},
		{
			name: "request_changes wins over others",
			results: []*ChunkResult{
				{
					Index: 0,
					Response: &ClaudeResponse{
						Summary:  "Part 1",
						Approval: "approve",
					},
				},
				{
					Index: 1,
					Response: &ClaudeResponse{
						Summary:  "Part 2 has issues",
						Approval: "request_changes",
					},
				},
				{
					Index: 2,
					Response: &ClaudeResponse{
						Summary:  "Part 3",
						Approval: "comment",
					},
				},
			},
			wantApproval: "request_changes",
			wantComments: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := MergeChunkResponses(tt.results)
			if err != nil {
				t.Fatalf("MergeChunkResponses() error = %v", err)
			}
			if merged.Approval != tt.wantApproval {
				t.Errorf("Approval = %q, want %q", merged.Approval, tt.wantApproval)
			}
			if len(merged.Comments) != tt.wantComments {
				t.Errorf("got %d comments, want %d", len(merged.Comments), tt.wantComments)
			}
		})
	}
}

func TestMergeApproval(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{"approve", "approve", "approve"},
		{"approve", "comment", "comment"},
		{"comment", "approve", "comment"},
		{"approve", "request_changes", "request_changes"},
		{"request_changes", "approve", "request_changes"},
		{"comment", "request_changes", "request_changes"},
		{"request_changes", "comment", "request_changes"},
		{"request_changes", "request_changes", "request_changes"},
	}

	for _, tt := range tests {
		got := mergeApproval(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("mergeApproval(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{100, "100"},
		{-5, "-5"},
	}

	for _, tt := range tests {
		got := itoa(tt.n)
		if got != tt.want {
			t.Errorf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		n        int
		singular string
		want     string
	}{
		{1, "file", "1 file"},
		{2, "file", "2 files"},
		{0, "file", "0 files"},
		{1, "chunk", "1 chunk"},
		{5, "chunk", "5 chunks"},
	}

	for _, tt := range tests {
		got := pluralize(tt.n, tt.singular)
		if got != tt.want {
			t.Errorf("pluralize(%d, %q) = %q, want %q", tt.n, tt.singular, got, tt.want)
		}
	}
}
