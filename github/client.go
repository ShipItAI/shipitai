package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
)

const (
	baseURL = "https://api.github.com"
)

// Client provides methods to interact with the GitHub API.
type Client struct {
	httpClient *http.Client
	appID      int64
	privateKey []byte
}

// NewClient creates a new GitHub API client.
// The privateKey should be the PEM-encoded private key of the GitHub App.
func NewClient(appID int64, privateKey []byte) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		appID:      appID,
		privateKey: privateKey,
	}
}

// getInstallationClient returns an HTTP client authenticated for the given installation.
func (c *Client) getInstallationClient(installationID int64) (*http.Client, error) {
	transport, err := ghinstallation.New(http.DefaultTransport, c.appID, installationID, c.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create installation transport: %w", err)
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

// FetchDiff fetches the diff for a pull request.
func (c *Client) FetchDiff(ctx context.Context, installationID int64, owner, repo string, prNumber int) (string, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", baseURL, owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.diff")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch diff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to fetch diff: status %d, body: %s", resp.StatusCode, string(body))
	}

	diff, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read diff: %w", err)
	}

	return string(diff), nil
}

// FetchPullRequestFiles fetches the list of files changed in a pull request.
func (c *Client) FetchPullRequestFiles(ctx context.Context, installationID int64, owner, repo string, prNumber int) ([]PullRequestFile, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files", baseURL, owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch files: status %d, body: %s", resp.StatusCode, string(body))
	}

	var files []PullRequestFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode files: %w", err)
	}

	return files, nil
}

// FetchFileContent fetches the content of a file from a repository.
func (c *Client) FetchFileContent(ctx context.Context, installationID int64, owner, repo, path, ref string) (string, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", baseURL, owner, repo, path, ref)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil // File doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to fetch file: status %d, body: %s", resp.StatusCode, string(body))
	}

	var content FileContent
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return "", fmt.Errorf("failed to decode file content: %w", err)
	}

	if content.Encoding != "base64" {
		return "", fmt.Errorf("unsupported encoding: %s", content.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(content.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 content: %w", err)
	}

	return string(decoded), nil
}

// CreateReview posts a review on a pull request.
func (c *Client) CreateReview(ctx context.Context, installationID int64, owner, repo string, prNumber int, review *ReviewRequest) (*Review, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", baseURL, owner, repo, prNumber)

	body, err := json.Marshal(review)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal review: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create review: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create review: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var createdReview Review
	if err := json.NewDecoder(resp.Body).Decode(&createdReview); err != nil {
		return nil, fmt.Errorf("failed to decode review response: %w", err)
	}

	return &createdReview, nil
}

// GetPullRequest fetches a pull request by number.
func (c *Client) GetPullRequest(ctx context.Context, installationID int64, owner, repo string, prNumber int) (*PullRequest, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", baseURL, owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch pull request: status %d, body: %s", resp.StatusCode, string(body))
	}

	var pr PullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("failed to decode pull request: %w", err)
	}

	return &pr, nil
}

// CreateReplyComment posts a reply to a review comment.
func (c *Client) CreateReplyComment(ctx context.Context, installationID int64, owner, repo string, prNumber int, commentID int64, body string) (*PullRequestComment, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments/%d/replies", baseURL, owner, repo, prNumber, commentID)

	reqBody, err := json.Marshal(CommentReply{Body: body})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reply: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create reply: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create reply: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var comment PullRequestComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("failed to decode reply response: %w", err)
	}

	return &comment, nil
}

// GetReviewComments fetches all review comments for a pull request.
func (c *Client) GetReviewComments(ctx context.Context, installationID int64, owner, repo string, prNumber int) ([]PullRequestComment, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments", baseURL, owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch comments: status %d, body: %s", resp.StatusCode, string(body))
	}

	var comments []PullRequestComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("failed to decode comments: %w", err)
	}

	return comments, nil
}

// FetchFileCommits fetches recent commits for a specific file.
func (c *Client) FetchFileCommits(ctx context.Context, installationID int64, owner, repo, path, ref string, limit int) ([]Commit, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	// Build URL with query parameters
	params := url.Values{}
	params.Set("path", path)
	params.Set("per_page", fmt.Sprintf("%d", limit))
	if ref != "" {
		params.Set("sha", ref)
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/commits?%s", baseURL, owner, repo, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commits: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // File has no commits (new file)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch commits: status %d, body: %s", resp.StatusCode, string(body))
	}

	var commits []Commit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, fmt.Errorf("failed to decode commits: %w", err)
	}

	return commits, nil
}

// UserPermission represents a user's permission level on a repository.
type UserPermission struct {
	Permission string `json:"permission"` // admin, write, read, none
	User       *User  `json:"user"`
}

// GetUserPermission gets a user's permission level for a repository.
// Returns "admin", "write", "read", or "none".
// Returns "none" if the user is not a collaborator (404 response).
func (c *Client) GetUserPermission(ctx context.Context, installationID int64, owner, repo, username string) (string, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/collaborators/%s/permission", baseURL, owner, repo, username)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get user permission: %w", err)
	}
	defer resp.Body.Close()

	// 404 means user is not a collaborator
	if resp.StatusCode == http.StatusNotFound {
		return "none", nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get user permission: status %d, body: %s", resp.StatusCode, string(body))
	}

	var perm UserPermission
	if err := json.NewDecoder(resp.Body).Decode(&perm); err != nil {
		return "", fmt.Errorf("failed to decode permission: %w", err)
	}

	return perm.Permission, nil
}

// IsContributor checks if a user has write or admin access to a repository.
// On API error, returns true (fail open) to avoid blocking legitimate PRs.
func (c *Client) IsContributor(ctx context.Context, installationID int64, owner, repo, username string) (bool, error) {
	permission, err := c.GetUserPermission(ctx, installationID, owner, repo, username)
	if err != nil {
		// Fail open on API errors to not block legitimate contributors
		return true, err
	}
	return permission == "admin" || permission == "write", nil
}

// IssueCommentRequest represents a request to create an issue comment.
type IssueCommentRequest struct {
	Body string `json:"body"`
}

// IssueCommentResponse represents a created issue comment.
type IssueCommentResponse struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	User    *User  `json:"user"`
}

// CreateIssueComment posts a comment on a PR (via the issues API).
func (c *Client) CreateIssueComment(ctx context.Context, installationID int64, owner, repo string, prNumber int, body string) (*IssueCommentResponse, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", baseURL, owner, repo, prNumber)

	reqBody, err := json.Marshal(IssueCommentRequest{Body: body})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal comment: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create comment: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var comment IssueCommentResponse
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("failed to decode comment response: %w", err)
	}

	return &comment, nil
}

// ListPRReviews fetches all reviews for a pull request.
func (c *Client) ListPRReviews(ctx context.Context, installationID int64, owner, repo string, prNumber int) ([]Review, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", baseURL, owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reviews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch reviews: status %d, body: %s", resp.StatusCode, string(body))
	}

	var reviews []Review
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return nil, fmt.Errorf("failed to decode reviews: %w", err)
	}

	return reviews, nil
}

// UpdateReviewBody updates the body text of an existing review.
// This only updates the summary comment, not inline comments.
func (c *Client) UpdateReviewBody(ctx context.Context, installationID int64, owner, repo string, prNumber int, reviewID int64, body string) error {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d", baseURL, owner, repo, prNumber, reviewID)

	reqBody, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update review: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update review: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// FetchMultipleFiles fetches multiple files in parallel.
// Returns a map of path -> content. Missing files are not included in the map.
func (c *Client) FetchMultipleFiles(ctx context.Context, installationID int64, owner, repo string, paths []string, ref string) (map[string]string, error) {
	result := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrent requests to avoid rate limiting
	sem := make(chan struct{}, 10)

	for _, path := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			content, err := c.FetchFileContent(ctx, installationID, owner, repo, p, ref)
			if err != nil {
				// Log but don't fail - missing files are expected
				return
			}
			if content != "" {
				mu.Lock()
				result[p] = content
				mu.Unlock()
			}
		}(path)
	}

	wg.Wait()
	return result, nil
}
