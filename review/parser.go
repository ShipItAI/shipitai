package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/shipitai/shipitai/github"
)

// AnnotateDiffWithLineNumbers adds new-file line numbers to each diff line
// so that Claude can directly read the correct line number instead of computing it.
// Context and added lines get a "NNNN | " prefix; deleted lines get "     | " (no number).
// File headers, hunk headers, and metadata lines pass through unchanged.
func AnnotateDiffWithLineNumbers(diff string) string {
	var result strings.Builder
	var currentLine int
	var inHunk bool

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		// New file section resets hunk state
		if strings.HasPrefix(line, "diff --git") {
			inHunk = false
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}

		// Hunk header — extract new-file starting line
		if matches := hunkHeaderRegex.FindStringSubmatch(line); matches != nil {
			startLine, _ := strconv.Atoi(matches[3])
			currentLine = startLine
			inHunk = true
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}

		// File metadata lines pass through unchanged
		if !inHunk {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}

		// Inside a hunk: annotate lines with new-file line numbers
		if strings.HasPrefix(line, "-") {
			// Deleted line — no new-file line number
			fmt.Fprintf(&result, "      | %s\n", line)
		} else if strings.HasPrefix(line, "+") {
			// Added line
			fmt.Fprintf(&result, "%5d | %s\n", currentLine, line)
			currentLine++
		} else if strings.HasPrefix(line, "\\") {
			// "\ No newline at end of file"
			result.WriteString(line)
			result.WriteByte('\n')
		} else {
			// Context line (starts with space) or blank line
			fmt.Fprintf(&result, "%5d | %s\n", currentLine, line)
			currentLine++
		}
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// DiffLineMap maps file paths to their valid commentable line numbers.
// A line is commentable if it appears in a diff hunk on the RIGHT side
// (i.e., added lines or context lines in the new version of the file).
type DiffLineMap map[string]map[int]bool

// hunkHeaderRegex matches unified diff hunk headers like "@@ -10,5 +15,7 @@"
var hunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseDiffLines parses a unified diff and returns a map of valid commentable lines.
// For each file, it tracks which line numbers in the NEW version appear in diff hunks.
func ParseDiffLines(diff string) DiffLineMap {
	result := make(DiffLineMap)

	var currentFile string
	var currentLine int
	var inHunk bool

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		// New file in diff
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = strings.TrimPrefix(line, "+++ b/")
			if result[currentFile] == nil {
				result[currentFile] = make(map[int]bool)
			}
			inHunk = false
			continue
		}

		// Handle +++ /dev/null for deleted files
		if strings.HasPrefix(line, "+++ /dev/null") {
			currentFile = ""
			inHunk = false
			continue
		}

		// Hunk header
		if matches := hunkHeaderRegex.FindStringSubmatch(line); matches != nil {
			if currentFile == "" {
				continue
			}
			// matches[3] is the starting line in the new file
			startLine, _ := strconv.Atoi(matches[3])
			currentLine = startLine
			inHunk = true
			continue
		}

		// Skip if not in a hunk or no current file
		if !inHunk || currentFile == "" {
			continue
		}

		// Process diff lines within a hunk
		if strings.HasPrefix(line, "-") {
			// Deleted line - doesn't exist in new file, don't increment
			continue
		} else if strings.HasPrefix(line, "+") {
			// Added line - exists in new file at currentLine
			result[currentFile][currentLine] = true
			currentLine++
		} else if strings.HasPrefix(line, " ") || line == "" {
			// Context line - exists in new file at currentLine
			result[currentFile][currentLine] = true
			currentLine++
		} else if strings.HasPrefix(line, "\\") {
			// "\ No newline at end of file" - ignore
			continue
		} else if strings.HasPrefix(line, "diff --git") {
			// New file section starting
			inHunk = false
		}
	}

	return result
}

// IsValidCommentLine checks if a line number is valid for commenting in a file.
func (m DiffLineMap) IsValidCommentLine(path string, line int) bool {
	fileLines, ok := m[path]
	if !ok {
		return false
	}
	return fileLines[line]
}

// FilterValidComments filters out comments with invalid line numbers.
// Returns the valid comments and a count of how many were filtered out.
func FilterValidComments(comments []ClaudeComment, diffLines DiffLineMap, logger *slog.Logger) ([]ClaudeComment, int) {
	if len(comments) == 0 {
		return comments, 0
	}

	valid := make([]ClaudeComment, 0, len(comments))
	filtered := 0

	for _, c := range comments {
		if diffLines.IsValidCommentLine(c.Path, c.Line) {
			valid = append(valid, c)
		} else {
			filtered++
			if logger != nil {
				logger.Warn("filtered comment with invalid line number",
					"path", c.Path,
					"line", c.Line,
					"body_preview", truncateString(c.Body, 50),
				)
			}
		}
	}

	return valid, filtered
}

// truncateString truncates a string to maxLen and adds "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ClaudeResponse represents Claude's structured review response.
type ClaudeResponse struct {
	Summary         string          `json:"summary"`
	Comments        []ClaudeComment `json:"comments"`
	Approval        string          `json:"approval"`
	ResolvedThreads []string        `json:"resolved_threads,omitempty"`
}

// ClaudeComment represents a single comment from Claude's review.
type ClaudeComment struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Body     string `json:"body"`
	Severity string `json:"severity,omitempty"` // "critical", "high", "medium", "low"
}

// ParseResponse parses Claude's JSON response into a structured review.
func ParseResponse(response string) (*ClaudeResponse, error) {
	// Clean up the response - remove markdown code blocks if present
	cleaned := cleanResponse(response)

	var result ClaudeResponse
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("failed to parse Claude response as JSON: %w\nResponse: %s", err, cleaned)
	}

	if err := validateResponse(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// cleanResponse removes markdown code blocks, preamble text, and other formatting
// to extract the JSON object from Claude's response.
func cleanResponse(response string) string {
	response = strings.TrimSpace(response)

	// Try to extract JSON from a ```json code block anywhere in the response
	if idx := strings.Index(response, "```json"); idx >= 0 {
		inner := response[idx+len("```json"):]
		if endIdx := strings.Index(inner, "```"); endIdx >= 0 {
			return strings.TrimSpace(inner[:endIdx])
		}
		return strings.TrimSpace(inner)
	}

	// Try a plain ``` code block anywhere in the response
	// But skip ``` blocks that are inside JSON (e.g., suggestion blocks in comment bodies)
	if idx := strings.Index(response, "```"); idx >= 0 {
		// Only use this if the ``` appears before any JSON opening brace
		jsonStart := strings.Index(response, "{")
		if jsonStart < 0 || idx < jsonStart {
			inner := response[idx+len("```"):]
			if endIdx := strings.Index(inner, "```"); endIdx >= 0 {
				return strings.TrimSpace(inner[:endIdx])
			}
			return strings.TrimSpace(inner)
		}
	}

	// No code blocks — strip any preamble text before the JSON object
	// Look for the start of the JSON response by finding {"summary" or {"comments" patterns first
	for _, marker := range []string{`{"summary"`, `{ "summary"`, `{"comments"`, `{ "comments"`} {
		if idx := strings.Index(response, marker); idx >= 0 {
			if lastBrace := strings.LastIndex(response, "}"); lastBrace > idx {
				return strings.TrimSpace(response[idx : lastBrace+1])
			}
		}
	}

	// Fall back to the outermost { ... } extraction
	if idx := strings.Index(response, "{"); idx >= 0 {
		if lastBrace := strings.LastIndex(response, "}"); lastBrace > idx {
			return strings.TrimSpace(response[idx : lastBrace+1])
		}
	}

	return strings.TrimSpace(response)
}

// validateResponse validates the parsed response.
func validateResponse(resp *ClaudeResponse) error {
	switch resp.Approval {
	case "approve", "request_changes", "comment":
		// Valid
	case "":
		resp.Approval = "comment" // Default to comment if empty
	default:
		return fmt.Errorf("invalid approval value: %s", resp.Approval)
	}

	for i, comment := range resp.Comments {
		if comment.Path == "" {
			return fmt.Errorf("comment %d has empty path", i)
		}
		if comment.Line <= 0 {
			return fmt.Errorf("comment %d has invalid line number: %d", i, comment.Line)
		}
		if comment.Body == "" {
			return fmt.Errorf("comment %d has empty body", i)
		}
		// Validate and normalize severity
		switch comment.Severity {
		case "critical", "high", "medium", "low":
			// Valid
		case "":
			resp.Comments[i].Severity = "medium" // Default to medium
		default:
			return fmt.Errorf("comment %d has invalid severity: %s (must be critical, high, medium, or low)", i, comment.Severity)
		}
	}

	return nil
}

// ToGitHubReview converts a ClaudeResponse to a GitHub review request.
func ToGitHubReview(resp *ClaudeResponse, commitSHA string) (*github.ReviewRequest, error) {
	if resp == nil {
		return nil, errors.New("nil response")
	}

	event := mapApprovalToEvent(resp.Approval)

	comments := make([]github.ReviewComment, len(resp.Comments))
	for i, c := range resp.Comments {
		comments[i] = github.ReviewComment{
			Path: c.Path,
			Line: c.Line,
			Side: "RIGHT", // Comments on the new version of the file
			Body: c.Body,
		}
	}

	return &github.ReviewRequest{
		CommitID: commitSHA,
		Body:     resp.Summary,
		Event:    event,
		Comments: comments,
	}, nil
}

// mapApprovalToEvent maps Claude's approval value to GitHub's event type.
func mapApprovalToEvent(approval string) string {
	switch approval {
	case "approve":
		return "APPROVE"
	case "request_changes":
		return "REQUEST_CHANGES"
	default:
		return "COMMENT"
	}
}

// BuildNonContributorMessage returns the message to post when a non-contributor opens a PR.
// This informs them that automatic reviews are only triggered for contributors.
func BuildNonContributorMessage(botName string) string {
	return fmt.Sprintf(`Thanks for opening this PR! To protect against abuse, automatic reviews are only triggered for repository contributors.

A contributor can trigger a review by commenting: `+"`@%s review`"+`

---
*[ShipItAI](https://shipitai.dev) - AI Code Reviews*`, botName)
}

// BuildUnauthorizedTriggerMessage returns the message when a non-contributor tries to trigger a review.
func BuildUnauthorizedTriggerMessage() string {
	return "Only repository contributors can trigger reviews. If you believe you should have access, please contact a repository maintainer."
}

// DetermineApprovalFromSeverity determines approval based on comment severities.
// Returns "request_changes" if there are critical or high severity comments.
// Returns "comment" if there are medium severity comments.
// Returns "approve" if there are only low severity comments or no comments.
func DetermineApprovalFromSeverity(comments []ClaudeComment) string {
	if len(comments) == 0 {
		return "approve"
	}

	for _, c := range comments {
		if c.Severity == "critical" || c.Severity == "high" {
			return "request_changes"
		}
	}

	for _, c := range comments {
		if c.Severity == "medium" {
			return "comment"
		}
	}

	return "approve"
}

// HasUnresolvedBlockers checks if there are any unresolved critical/high severity comments.
// Used when deciding whether to approve after subsequent reviews.
func HasUnresolvedBlockers(comments []ClaudeComment) bool {
	for _, c := range comments {
		if c.Severity == "critical" || c.Severity == "high" {
			return true
		}
	}
	return false
}

// FormatCommentWithSeverity adds a severity badge to the comment body.
func FormatCommentWithSeverity(body, severity string) string {
	switch severity {
	case "critical":
		return "**[critical]** " + body
	case "high":
		return "**[high]** " + body
	case "low":
		return "*[low]* " + body
	default:
		return body
	}
}
