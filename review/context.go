// Package review provides code review functionality using Claude.
package review

// FileContext represents the full content of a file being reviewed.
type FileContext struct {
	// Path is the file path relative to the repository root.
	Path string
	// Content is the full file content.
	Content string
	// Language is the detected programming language (e.g., "go", "typescript", "python").
	Language string
	// Truncated indicates if the content was truncated due to size limits.
	Truncated bool
}

// RelatedFile represents a file related to a modified file.
type RelatedFile struct {
	// Path is the file path relative to the repository root.
	Path string
	// Relationship describes how this file relates to the modified file.
	// Values: "test", "import"
	Relationship string
	// Content is the full file content.
	Content string
	// SourceFile is the modified file this is related to.
	SourceFile string
}

// CommitInfo contains information about a single commit.
type CommitInfo struct {
	// SHA is the commit hash.
	SHA string
	// Message is the first line of the commit message.
	Message string
	// Author is the commit author's login or name.
	Author string
}

// FileHistory contains the recent commit history for a file.
type FileHistory struct {
	// Path is the file path.
	Path string
	// Commits is the list of recent commits that modified this file.
	Commits []CommitInfo
}

// ReviewContext holds all the enriched context for a code review.
// This is fetched on-demand and never stored.
type ReviewContext struct {
	// FullFiles contains the complete content of modified files.
	FullFiles []FileContext
	// RelatedFiles contains test files and imported files.
	RelatedFiles []RelatedFile
	// FileHistories contains recent commit history per modified file.
	FileHistories []FileHistory
}

// IsEmpty returns true if no context was fetched.
func (c *ReviewContext) IsEmpty() bool {
	return len(c.FullFiles) == 0 && len(c.RelatedFiles) == 0 && len(c.FileHistories) == 0
}

// TotalSize returns the approximate size of all content in bytes.
func (c *ReviewContext) TotalSize() int {
	size := 0
	for _, f := range c.FullFiles {
		size += len(f.Content)
	}
	for _, f := range c.RelatedFiles {
		size += len(f.Content)
	}
	return size
}
