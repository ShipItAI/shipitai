// Package github provides GitHub API client and webhook handling for the reviewer.
package github

import "time"

// WebhookEvent represents a GitHub webhook event.
type WebhookEvent struct {
	Action       string       `json:"action"`
	Number       int          `json:"number"`
	PullRequest  *PullRequest `json:"pull_request,omitempty"`
	Repository   *Repository  `json:"repository"`
	Installation *Installation `json:"installation"`
	Sender       *User        `json:"sender"`
}

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	ID        int64  `json:"id"`
	Number    int    `json:"number"`
	State     string `json:"state"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Head      *Ref   `json:"head"`
	Base      *Ref   `json:"base"`
	User      *User  `json:"user"`
	HTMLURL   string `json:"html_url"`
	DiffURL   string `json:"diff_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Ref represents a git reference (branch/commit).
type Ref struct {
	Ref  string      `json:"ref"`
	SHA  string      `json:"sha"`
	Repo *Repository `json:"repo,omitempty"`
}

// Repository represents a GitHub repository.
type Repository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Owner         *User  `json:"owner"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
	HTMLURL       string `json:"html_url"`
}

// User represents a GitHub user or organization.
type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"`
}

// Installation represents a GitHub App installation.
type Installation struct {
	ID int64 `json:"id"`
}

// PullRequestFile represents a file changed in a pull request.
type PullRequestFile struct {
	SHA              string `json:"sha"`
	Filename         string `json:"filename"`
	Status           string `json:"status"` // added, removed, modified, renamed, copied, changed, unchanged
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch,omitempty"`
	PreviousFilename string `json:"previous_filename,omitempty"`
}

// ReviewComment represents a comment on a specific line in a pull request review.
type ReviewComment struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side,omitempty"` // LEFT or RIGHT, defaults to RIGHT
	Body     string `json:"body"`
	Position int    `json:"position,omitempty"` // deprecated, use line instead
}

// ReviewRequest represents a request to create a pull request review.
type ReviewRequest struct {
	CommitID string          `json:"commit_id,omitempty"`
	Body     string          `json:"body"`
	Event    string          `json:"event"` // APPROVE, REQUEST_CHANGES, COMMENT
	Comments []ReviewComment `json:"comments,omitempty"`
}

// Review represents a pull request review response.
type Review struct {
	ID          int64     `json:"id"`
	NodeID      string    `json:"node_id"`
	User        *User     `json:"user"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	HTMLURL     string    `json:"html_url"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// FileContent represents the content of a file from the GitHub API.
type FileContent struct {
	Type        string `json:"type"`
	Encoding    string `json:"encoding"`
	Size        int    `json:"size"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Content     string `json:"content"`
	SHA         string `json:"sha"`
	URL         string `json:"url"`
	GitURL      string `json:"git_url"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
}

// ReviewCommentEvent represents a pull_request_review_comment webhook event.
type ReviewCommentEvent struct {
	Action       string                `json:"action"` // created, edited, deleted
	Comment      *PullRequestComment   `json:"comment"`
	PullRequest  *PullRequest          `json:"pull_request"`
	Repository   *Repository           `json:"repository"`
	Installation *Installation         `json:"installation"`
	Sender       *User                 `json:"sender"`
}

// PullRequestComment represents a comment on a pull request review.
type PullRequestComment struct {
	ID                  int64  `json:"id"`
	NodeID              string `json:"node_id"`
	PullRequestReviewID int64  `json:"pull_request_review_id"`
	DiffHunk            string `json:"diff_hunk"`
	Path                string `json:"path"`
	Position            int    `json:"position,omitempty"`
	OriginalPosition    int    `json:"original_position,omitempty"`
	CommitID            string `json:"commit_id"`
	OriginalCommitID    string `json:"original_commit_id"`
	InReplyToID         int64  `json:"in_reply_to_id,omitempty"`
	User                *User  `json:"user"`
	Body                string `json:"body"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
	HTMLURL             string `json:"html_url"`
	Line                int    `json:"line,omitempty"`
	OriginalLine        int    `json:"original_line,omitempty"`
	StartLine           int    `json:"start_line,omitempty"`
	OriginalStartLine   int    `json:"original_start_line,omitempty"`
	Side                string `json:"side,omitempty"`
	StartSide           string `json:"start_side,omitempty"`
}

// CommentReply represents a reply to a review comment.
type CommentReply struct {
	Body string `json:"body"`
}

// InstallationEvent represents an installation webhook event.
type InstallationEvent struct {
	Action       string        `json:"action"` // created, deleted, suspend, unsuspend
	Installation *InstallationDetails `json:"installation"`
	Sender       *User         `json:"sender"`
}

// InstallationDetails contains details about a GitHub App installation.
type InstallationDetails struct {
	ID      int64  `json:"id"`
	Account *User  `json:"account"` // The org or user that installed the app
}

// Commit represents a commit from the GitHub API.
type Commit struct {
	SHA    string        `json:"sha"`
	Commit *CommitDetail `json:"commit"`
	Author *User         `json:"author,omitempty"` // GitHub user (may be nil for non-users)
}

// CommitDetail contains the commit details.
type CommitDetail struct {
	Message string        `json:"message"`
	Author  *CommitAuthor `json:"author,omitempty"`
}

// CommitAuthor contains commit author information.
type CommitAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

// IssueCommentEvent represents an issue_comment webhook event.
// This is used for PR comments triggered via the issues API (not review comments).
type IssueCommentEvent struct {
	Action       string        `json:"action"` // created, edited, deleted
	Issue        *Issue        `json:"issue"`
	Comment      *IssueComment `json:"comment"`
	Repository   *Repository   `json:"repository"`
	Installation *Installation `json:"installation"`
	Sender       *User         `json:"sender"`
}

// Issue represents a GitHub issue (PRs are also issues).
type Issue struct {
	ID          int64         `json:"id"`
	Number      int           `json:"number"`
	Title       string        `json:"title"`
	Body        string        `json:"body"`
	State       string        `json:"state"`
	User        *User         `json:"user"`
	PullRequest *IssuePRLink  `json:"pull_request,omitempty"` // Non-nil if this issue is a PR
	HTMLURL     string        `json:"html_url"`
}

// IssuePRLink contains PR-specific URLs when an issue is a PR.
type IssuePRLink struct {
	URL      string `json:"url"`
	HTMLURL  string `json:"html_url"`
	DiffURL  string `json:"diff_url"`
	PatchURL string `json:"patch_url"`
}

// IssueComment represents a comment on an issue or PR.
type IssueComment struct {
	ID        int64  `json:"id"`
	NodeID    string `json:"node_id"`
	User      *User  `json:"user"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	HTMLURL   string `json:"html_url"`
}
