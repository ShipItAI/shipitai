package review

import (
	"strings"
)

// ChunkThreshold is the diff size (in bytes) above which chunking is triggered.
// ~100KB corresponds to roughly 25K tokens.
const ChunkThreshold = 100 * 1024

// MaxChunkSize is the maximum size (in bytes) for a single chunk.
// ~80KB corresponds to roughly 20K tokens.
const MaxChunkSize = 80 * 1024

// FileDiff represents a single file's diff content.
type FileDiff struct {
	Path    string
	Content string
}

// Chunk represents a group of file diffs to be reviewed together.
type Chunk struct {
	Files     []FileDiff
	SizeBytes int
	Index     int
	Total     int
}

// ChunkResult holds the response from reviewing a single chunk.
type ChunkResult struct {
	Response *ClaudeResponse
	Index    int
}

// MergedReview represents the combined result of all chunk reviews.
type MergedReview struct {
	Summary  string
	Comments []ClaudeComment
	Approval string
}

// SplitDiffByFile splits a unified diff into individual file diffs.
// Each FileDiff contains the complete diff for a single file.
func SplitDiffByFile(diff string) []FileDiff {
	if diff == "" {
		return nil
	}

	var files []FileDiff
	var currentFile FileDiff
	var content strings.Builder

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			// Save previous file if we have one
			if currentFile.Path != "" {
				currentFile.Content = strings.TrimSuffix(content.String(), "\n")
				files = append(files, currentFile)
				content.Reset()
			}

			// Extract file path from "diff --git a/path b/path"
			parts := strings.Split(line, " ")
			if len(parts) >= 4 {
				currentFile = FileDiff{
					Path: strings.TrimPrefix(parts[3], "b/"),
				}
			} else {
				currentFile = FileDiff{Path: "unknown"}
			}
		}

		content.WriteString(line)
		content.WriteString("\n")
	}

	// Don't forget the last file
	if currentFile.Path != "" {
		currentFile.Content = strings.TrimSuffix(content.String(), "\n")
		files = append(files, currentFile)
	}

	return files
}

// ChunkDiff splits a diff into chunks that fit within the size limit.
// Uses greedy bin-packing: adds files to current chunk until limit reached.
func ChunkDiff(diff string, maxChunkSize int) []Chunk {
	files := SplitDiffByFile(diff)
	if len(files) == 0 {
		return nil
	}

	var chunks []Chunk
	var currentChunk Chunk
	currentChunk.Files = make([]FileDiff, 0)

	for _, file := range files {
		fileSize := len(file.Content)

		// If single file exceeds max size, it gets its own chunk
		if fileSize > maxChunkSize {
			// Save current chunk if it has files
			if len(currentChunk.Files) > 0 {
				chunks = append(chunks, currentChunk)
				currentChunk = Chunk{Files: make([]FileDiff, 0)}
			}
			// Add oversized file as its own chunk
			chunks = append(chunks, Chunk{
				Files:     []FileDiff{file},
				SizeBytes: fileSize,
			})
			continue
		}

		// Would adding this file exceed the limit?
		if currentChunk.SizeBytes+fileSize > maxChunkSize && len(currentChunk.Files) > 0 {
			// Save current chunk and start new one
			chunks = append(chunks, currentChunk)
			currentChunk = Chunk{Files: make([]FileDiff, 0)}
		}

		// Add file to current chunk
		currentChunk.Files = append(currentChunk.Files, file)
		currentChunk.SizeBytes += fileSize
	}

	// Don't forget the last chunk
	if len(currentChunk.Files) > 0 {
		chunks = append(chunks, currentChunk)
	}

	// Set index and total on all chunks
	total := len(chunks)
	for i := range chunks {
		chunks[i].Index = i
		chunks[i].Total = total
	}

	return chunks
}

// ChunkToDiff converts a Chunk back to a unified diff string.
func ChunkToDiff(chunk *Chunk) string {
	var builder strings.Builder
	for i, file := range chunk.Files {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(file.Content)
	}
	return builder.String()
}

// MergeChunkResponses combines responses from multiple chunks into a single review.
// Approval uses strictest-wins: request_changes > comment > approve
func MergeChunkResponses(results []*ChunkResult) (*MergedReview, error) {
	if len(results) == 0 {
		return &MergedReview{
			Summary:  "No chunks to review.",
			Approval: "comment",
		}, nil
	}

	merged := &MergedReview{
		Comments: make([]ClaudeComment, 0),
		Approval: "approve", // Start with least strict
	}

	var summaries []string
	fileGroupCount := 0

	// Process results in order (they should already be sorted by index)
	for _, result := range results {
		if result == nil || result.Response == nil {
			continue
		}

		resp := result.Response
		fileGroupCount++

		// Collect summaries
		if resp.Summary != "" {
			summaries = append(summaries, resp.Summary)
		}

		// Collect all comments
		merged.Comments = append(merged.Comments, resp.Comments...)

		// Merge approval (strictest wins)
		merged.Approval = mergeApproval(merged.Approval, resp.Approval)
	}

	// Build combined summary
	if len(summaries) == 1 {
		merged.Summary = summaries[0]
	} else if len(summaries) > 1 {
		var builder strings.Builder
		builder.WriteString("**Reviewed ")
		builder.WriteString(pluralize(fileGroupCount, "file group"))
		builder.WriteString(":**\n\n")
		for i, summary := range summaries {
			builder.WriteString("**Part ")
			builder.WriteString(itoa(i + 1))
			builder.WriteString(":** ")
			builder.WriteString(summary)
			if i < len(summaries)-1 {
				builder.WriteString("\n\n")
			}
		}
		merged.Summary = builder.String()
	}

	return merged, nil
}

// mergeApproval returns the stricter of two approval values.
// Order: request_changes > comment > approve
func mergeApproval(a, b string) string {
	priority := map[string]int{
		"approve":         0,
		"comment":         1,
		"request_changes": 2,
	}

	pa, oka := priority[a]
	pb, okb := priority[b]

	if !oka {
		pa = 1 // default to comment
	}
	if !okb {
		pb = 1
	}

	if pa >= pb {
		return a
	}
	return b
}

// itoa converts an int to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte
	neg := n < 0
	if neg {
		n = -n
	}

	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if neg {
		digits = append([]byte{'-'}, digits...)
	}

	return string(digits)
}

// pluralize returns "n thing" or "n things" based on count.
func pluralize(n int, singular string) string {
	if n == 1 {
		return "1 " + singular
	}
	return itoa(n) + " " + singular + "s"
}
