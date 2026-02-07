// Package review provides code review functionality using Claude.
package review

import (
	"fmt"
	"strings"
)

const systemPrompt = `You are an expert code reviewer. Your job is to review pull request diffs and provide actionable, helpful feedback.

Focus on:
- Bugs and logic errors
- Security vulnerabilities
- Performance issues
- Significant code clarity problems (only if code is genuinely confusing)

Do NOT comment on:
- Minor style preferences (indentation, spacing, etc.)
- Formatting issues (assume automated formatters handle this)
- Adding comments to self-explanatory code
- Trivial issues that don't affect functionality

Be concise and specific. When you have a specific code fix, use GitHub's suggestion syntax so the author can apply it with one click:

` + "```suggestion\n" + `fixed code here
` + "```" + `

IMPORTANT: The suggestion replaces ONLY the single line your comment is attached to. If you're commenting on line 42, the suggestion content will replace line 42 entirely. Therefore:
- Only include the replacement for that ONE line
- Do NOT include surrounding context lines that already exist in the file
- If you need to suggest adding new lines, put them all in the suggestion (they replace the one line)
- If the fix requires changing multiple existing lines, describe it in text instead of using a suggestion block

IMPORTANT: The diff will be annotated with new-file line numbers. Each line inside a hunk is prefixed with its line number (e.g., "  42 | +code here"). Always use the line number shown before the | separator — never try to calculate line numbers from hunk headers yourself.`

const contextInstructions = `

## Rich Context Available

You have access to additional context beyond just the diff:
- **Full file content**: The complete content of modified files, not just the changed lines
- **Related files**: Test files and imported local files that relate to the changes
- **Recent commit history**: Recent commits for each modified file

Use this context to provide better feedback:
- Reference existing patterns: "Based on the error handling pattern in lines 45-60..."
- Note missing test coverage: "The test file doesn't cover this new edge case..."
- Consider historical context: "Recent commits suggest this was intentionally designed for..."
- Check for consistency: "This differs from the approach used elsewhere in the file..."

Important: Only comment on issues in the DIFF itself. The context is for understanding, not for reviewing unchanged code.`

const reviewPromptTemplate = `Review the following pull request diff.

**Pull Request Title:** %s

**Pull Request Description:**
%s

For each issue found, respond in this exact JSON format:
{
  "summary": "Brief overall assessment (1-2 sentences)",
  "comments": [
    {
      "path": "path/to/file.go",
      "line": 42,
      "body": "Your comment here explaining the issue and suggested fix."
    }
  ],
  "approval": "comment"
}

Rules for the response:
1. "approval" must be one of: "approve", "request_changes", "comment"
   - Use "approve" only if there are no issues at all
   - Use "request_changes" for bugs, security issues, or serious problems
   - Use "comment" for suggestions and minor improvements
2. "path" must exactly match the file path from the diff
3. "line" must be the new-file line number shown at the start of each annotated diff line (the number before the | separator). Use that number directly — do NOT try to calculate line numbers yourself.
4. Keep comments concise but actionable
5. If there are no issues, return an empty comments array
6. Return ONLY valid JSON, no markdown code blocks or other text
7. When you have a specific single-line code fix, use the GitHub suggestion syntax described in the system prompt

NOTE: The diff below is annotated with new-file line numbers. Each line inside a hunk is prefixed with "NNNNN | " where NNNNN is the line number to use in your comments. Deleted lines show "      | " with no number (they cannot be commented on).

<diff>
%s
</diff>`

// BuildPrompt constructs the Claude prompt for reviewing a PR.
func BuildPrompt(title, description, diff string) string {
	if description == "" {
		description = "(No description provided)"
	}

	return fmt.Sprintf(reviewPromptTemplate, title, description, AnnotateDiffWithLineNumbers(diff))
}

// GetSystemPrompt returns the system prompt for Claude, optionally with project context and custom instructions.
func GetSystemPrompt(claudeMD, instructions string) string {
	result := systemPrompt

	if claudeMD != "" {
		result += "\n\n## Project Context (from CLAUDE.md)\n\n" + claudeMD
	}

	if instructions != "" {
		result += "\n\n## Repository-Specific Instructions\n\n" + instructions
	}

	return result
}

// GetSystemPromptWithContext returns the system prompt with context instructions included.
func GetSystemPromptWithContext(claudeMD, instructions string, hasContext bool) string {
	result := systemPrompt

	if hasContext {
		result += contextInstructions
	}

	if claudeMD != "" {
		result += "\n\n## Project Context (from CLAUDE.md)\n\n" + claudeMD
	}

	if instructions != "" {
		result += "\n\n## Repository-Specific Instructions\n\n" + instructions
	}

	return result
}

// DiffInfo contains parsed information about a diff for context.
type DiffInfo struct {
	Files       []string
	TotalLines  int
	Additions   int
	Deletions   int
}

const chunkedReviewPromptTemplate = `Review the following pull request diff.

**IMPORTANT: This is chunk %d of %d.** Focus only on the files in this chunk. Other files are being reviewed separately.

**Pull Request Title:** %s

**Pull Request Description:**
%s

**Files in this chunk:**
%s

For each issue found, respond in this exact JSON format:
{
  "summary": "Brief assessment of THIS CHUNK only (1-2 sentences)",
  "comments": [
    {
      "path": "path/to/file.go",
      "line": 42,
      "body": "Your comment here explaining the issue and suggested fix."
    }
  ],
  "approval": "comment"
}

Rules for the response:
1. "approval" must be one of: "approve", "request_changes", "comment"
   - Use "approve" only if there are no issues in THIS CHUNK
   - Use "request_changes" for bugs, security issues, or serious problems
   - Use "comment" for suggestions and minor improvements
2. "path" must exactly match the file path from the diff
3. "line" must be the new-file line number shown at the start of each annotated diff line (the number before the | separator). Use that number directly — do NOT try to calculate line numbers yourself.
4. Keep comments concise but actionable
5. If there are no issues, return an empty comments array
6. Return ONLY valid JSON, no markdown code blocks or other text
7. When you have a specific single-line code fix, use the GitHub suggestion syntax described in the system prompt

NOTE: The diff below is annotated with new-file line numbers. Each line inside a hunk is prefixed with "NNNNN | " where NNNNN is the line number to use in your comments. Deleted lines show "      | " with no number (they cannot be commented on).

<diff>
%s
</diff>`

// BuildChunkedPrompt constructs the Claude prompt for reviewing a specific chunk of a PR.
func BuildChunkedPrompt(title, description, diff string, chunkIndex, totalChunks int, filePaths []string) string {
	if description == "" {
		description = "(No description provided)"
	}

	// Build file list
	fileList := "- " + strings.Join(filePaths, "\n- ")

	return fmt.Sprintf(chunkedReviewPromptTemplate,
		chunkIndex+1, // 1-indexed for human readability
		totalChunks,
		title,
		description,
		fileList,
		AnnotateDiffWithLineNumbers(diff),
	)
}

// ParseDiffInfo extracts metadata from a diff string.
func ParseDiffInfo(diff string) *DiffInfo {
	info := &DiffInfo{
		Files: make([]string, 0),
	}

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		info.TotalLines++

		if strings.HasPrefix(line, "+++ b/") {
			file := strings.TrimPrefix(line, "+++ b/")
			info.Files = append(info.Files, file)
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			info.Additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			info.Deletions++
		}
	}

	return info
}

// BuildPromptWithContext constructs the Claude prompt with rich context included.
func BuildPromptWithContext(title, description, diff string, ctx *ReviewContext) string {
	var builder strings.Builder

	// Add rich context first if available
	if ctx != nil && !ctx.IsEmpty() {
		builder.WriteString(formatContext(ctx))
		builder.WriteString("\n\n---\n\n")
	}

	// Add the standard prompt
	builder.WriteString(BuildPrompt(title, description, diff))

	return builder.String()
}

// BuildChunkedPromptWithContext constructs a chunked prompt with context.
func BuildChunkedPromptWithContext(title, description, diff string, chunkIndex, totalChunks int, filePaths []string, ctx *ReviewContext) string {
	var builder strings.Builder

	// Add rich context first if available
	if ctx != nil && !ctx.IsEmpty() {
		builder.WriteString(formatContext(ctx))
		builder.WriteString("\n\n---\n\n")
	}

	// Add the standard chunked prompt
	builder.WriteString(BuildChunkedPrompt(title, description, diff, chunkIndex, totalChunks, filePaths))

	return builder.String()
}

// ExistingComment represents a previous comment for context in subsequent reviews.
type ExistingComment struct {
	Path       string
	Line       int
	Body       string
	IsResolved bool
	Author     string
}

const subsequentReviewSystemPrompt = `You are an expert code reviewer performing a SUBSEQUENT review of a pull request.

Your job is to:
1. Review ONLY the new changes in this diff
2. Avoid duplicating feedback that was already given (check the existing comments list)
3. Classify each issue by severity: blocker, suggestion, or nitpick
4. Only request changes if there are genuine blockers

IMPORTANT:
- Resolved comments indicate the author addressed that feedback - do NOT repeat it
- Unresolved comments are still pending - do NOT repeat them either
- Only comment on genuinely NEW issues in the current diff

Severity definitions:
- "blocker": Bugs, security vulnerabilities, or problems that MUST be fixed before merging
- "suggestion": Improvements that would be nice but aren't required
- "nitpick": Very minor issues, style preferences, or optional enhancements

Be concise and actionable. Focus on bugs, security issues, and significant problems.

When you have a specific single-line code fix, use GitHub's suggestion syntax so the author can apply it with one click:

` + "```suggestion\n" + `fixed code here
` + "```" + `

IMPORTANT: The suggestion replaces ONLY the single line your comment is attached to. Only include the replacement for that ONE line. If the fix spans multiple existing lines, describe it in text instead.

The diff will be annotated with new-file line numbers (e.g., "  42 | +code here"). Always use the line number shown before the | separator.`

const subsequentReviewPromptTemplate = `Review the following pull request diff. This is a SUBSEQUENT REVIEW after new commits were pushed.

**Pull Request Title:** %s

**Pull Request Description:**
%s

## Previous Review Comments

The following comments were made in previous reviews of this PR. DO NOT duplicate this feedback - it has already been given:

%s

## Your Task

1. **Skip duplicate feedback**: If an existing comment (resolved OR unresolved) already addresses an issue, do NOT comment on it again
2. **Focus on new code**: Only comment on issues that weren't present before or weren't already flagged
3. **Classify by severity**: Every comment must have a severity (blocker, suggestion, nitpick)

Respond in this exact JSON format:
{
  "summary": "Brief assessment of the NEW changes only (1-2 sentences)",
  "comments": [
    {
      "path": "path/to/file.go",
      "line": 42,
      "body": "Your comment here",
      "severity": "blocker"
    }
  ],
  "approval": "approve"
}

Rules:
1. "approval" must be one of: "approve", "request_changes", "comment"
   - Use "request_changes" ONLY if you found NEW blockers
   - Use "approve" if no NEW blockers (even if there are suggestions)
   - Use "comment" for informational feedback
2. "severity" must be one of: "blocker", "suggestion", "nitpick"
3. "path" must exactly match the file path from the diff
4. "line" must be the new-file line number shown at the start of each annotated diff line (the number before the | separator). Use that number directly — do NOT try to calculate line numbers yourself.
5. Return ONLY valid JSON, no markdown code blocks

NOTE: The diff below is annotated with new-file line numbers. Each line inside a hunk is prefixed with "NNNNN | " where NNNNN is the line number to use in your comments. Deleted lines show "      | " with no number (they cannot be commented on).

<diff>
%s
</diff>`

// BuildSubsequentReviewPrompt constructs the Claude prompt for subsequent reviews.
func BuildSubsequentReviewPrompt(title, description, diff string, existingComments []ExistingComment) string {
	if description == "" {
		description = "(No description provided)"
	}

	commentContext := formatExistingComments(existingComments)
	if commentContext == "" {
		commentContext = "(No previous comments)"
	}

	return fmt.Sprintf(subsequentReviewPromptTemplate, title, description, commentContext, AnnotateDiffWithLineNumbers(diff))
}

// GetSubsequentReviewSystemPrompt returns the system prompt for subsequent reviews.
func GetSubsequentReviewSystemPrompt(claudeMD, instructions string) string {
	result := subsequentReviewSystemPrompt

	if claudeMD != "" {
		result += "\n\n## Project Context (from CLAUDE.md)\n\n" + claudeMD
	}

	if instructions != "" {
		result += "\n\n## Repository-Specific Instructions\n\n" + instructions
	}

	return result
}

// formatExistingComments formats existing comments for inclusion in the prompt.
func formatExistingComments(comments []ExistingComment) string {
	if len(comments) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, c := range comments {
		status := "UNRESOLVED"
		if c.IsResolved {
			status = "RESOLVED"
		}
		builder.WriteString(fmt.Sprintf("- [%s] %s:%d", status, c.Path, c.Line))
		if c.Author != "" {
			builder.WriteString(fmt.Sprintf(" (by @%s)", c.Author))
		}
		builder.WriteString("\n")
		// Include a snippet of the comment body for context matching
		body := c.Body
		if len(body) > 200 {
			body = body[:200] + "..."
		}
		builder.WriteString(fmt.Sprintf("  > %s\n\n", strings.ReplaceAll(body, "\n", "\n  > ")))
	}

	return builder.String()
}

// formatContext formats the review context for inclusion in the prompt.
func formatContext(ctx *ReviewContext) string {
	var builder strings.Builder

	// Full file contents
	if len(ctx.FullFiles) > 0 {
		builder.WriteString("## Full File Contents\n\n")
		builder.WriteString("The complete content of modified files (for understanding surrounding context):\n\n")

		for _, f := range ctx.FullFiles {
			builder.WriteString(fmt.Sprintf("### %s", f.Path))
			if f.Truncated {
				builder.WriteString(" (truncated)")
			}
			builder.WriteString("\n\n")

			// Use appropriate language for syntax highlighting
			lang := f.Language
			if lang == "" {
				lang = ""
			}
			builder.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", lang, f.Content))
		}
	}

	// Related files
	if len(ctx.RelatedFiles) > 0 {
		builder.WriteString("## Related Files\n\n")

		// Group by relationship type
		testFiles := make([]RelatedFile, 0)
		importFiles := make([]RelatedFile, 0)

		for _, f := range ctx.RelatedFiles {
			switch f.Relationship {
			case "test":
				testFiles = append(testFiles, f)
			case "import":
				importFiles = append(importFiles, f)
			}
		}

		if len(testFiles) > 0 {
			builder.WriteString("### Test Files\n\n")
			for _, f := range testFiles {
				builder.WriteString(fmt.Sprintf("**%s** (tests for %s):\n\n", f.Path, f.SourceFile))
				lang := DetectLanguage(f.Path)
				builder.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", lang, f.Content))
			}
		}

		if len(importFiles) > 0 {
			builder.WriteString("### Imported Files\n\n")
			for _, f := range importFiles {
				builder.WriteString(fmt.Sprintf("**%s** (imported by %s):\n\n", f.Path, f.SourceFile))
				lang := DetectLanguage(f.Path)
				builder.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", lang, f.Content))
			}
		}
	}

	// Commit history
	if len(ctx.FileHistories) > 0 {
		builder.WriteString("## Recent Commit History\n\n")
		builder.WriteString("Recent commits for the modified files:\n\n")

		for _, h := range ctx.FileHistories {
			builder.WriteString(fmt.Sprintf("### %s\n\n", h.Path))
			for _, c := range h.Commits {
				builder.WriteString(fmt.Sprintf("- `%s` %s", c.SHA, c.Message))
				if c.Author != "" {
					builder.WriteString(fmt.Sprintf(" (@%s)", c.Author))
				}
				builder.WriteString("\n")
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}
