package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const graphQLURL = "https://api.github.com/graphql"

// ReviewThread represents a review thread with resolution status from GitHub's GraphQL API.
type ReviewThread struct {
	ID         string          `json:"id"`
	IsResolved bool            `json:"isResolved"`
	Path       string          `json:"path"`
	Line       int             `json:"line"`
	Comments   []ThreadComment `json:"comments"`
}

// ThreadComment represents a comment within a review thread.
type ThreadComment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	Author    string `json:"author"`
	CreatedAt string `json:"createdAt"`
}

// graphQLRequest represents a GraphQL query request.
type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// graphQLResponse represents the top-level GraphQL response structure.
type graphQLResponse struct {
	Data   *graphQLData   `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLData struct {
	Repository *graphQLRepository `json:"repository"`
}

type graphQLRepository struct {
	PullRequest *graphQLPullRequest `json:"pullRequest"`
}

type graphQLPullRequest struct {
	ReviewThreads *graphQLReviewThreads `json:"reviewThreads"`
}

type graphQLReviewThreads struct {
	Nodes    []graphQLReviewThread `json:"nodes"`
	PageInfo *graphQLPageInfo      `json:"pageInfo"`
}

type graphQLPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type graphQLReviewThread struct {
	ID         string              `json:"id"`
	IsResolved bool                `json:"isResolved"`
	Path       string              `json:"path"`
	Line       int                 `json:"line"`
	Comments   *graphQLComments    `json:"comments"`
}

type graphQLComments struct {
	Nodes []graphQLComment `json:"nodes"`
}

type graphQLComment struct {
	ID        string         `json:"id"`
	Body      string         `json:"body"`
	Author    *graphQLAuthor `json:"author"`
	CreatedAt string         `json:"createdAt"`
}

type graphQLAuthor struct {
	Login string `json:"login"`
}

const reviewThreadsQuery = `
query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $cursor) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          isResolved
          path
          line
          comments(first: 20) {
            nodes {
              id
              body
              author {
                login
              }
              createdAt
            }
          }
        }
      }
    }
  }
}
`

// maxPaginationPages is the maximum number of pages to fetch to prevent infinite loops.
const maxPaginationPages = 100

// FetchPRReviewThreads fetches all review threads for a PR with resolution status using GraphQL.
// This is needed because the REST API doesn't include resolution status.
func (c *Client) FetchPRReviewThreads(ctx context.Context, installationID int64, owner, repo string, prNumber int) ([]ReviewThread, error) {
	client, err := c.getInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	var allThreads []ReviewThread
	var cursor *string
	var prevCursor string

	for page := 0; page < maxPaginationPages; page++ {
		threads, pageInfo, err := c.fetchReviewThreadsPage(ctx, client, owner, repo, prNumber, cursor)
		if err != nil {
			return nil, err
		}

		allThreads = append(allThreads, threads...)

		// Handle pagination
		if pageInfo == nil || !pageInfo.HasNextPage {
			break
		}

		// Guard against API returning the same cursor (would cause infinite loop)
		if pageInfo.EndCursor == prevCursor {
			return nil, fmt.Errorf("GraphQL pagination stuck: cursor %q returned twice", pageInfo.EndCursor)
		}
		prevCursor = pageInfo.EndCursor
		cursor = &pageInfo.EndCursor
	}

	return allThreads, nil
}

// fetchReviewThreadsPage fetches a single page of review threads.
func (c *Client) fetchReviewThreadsPage(ctx context.Context, client *http.Client, owner, repo string, prNumber int, cursor *string) ([]ReviewThread, *graphQLPageInfo, error) {
	variables := map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": prNumber,
	}
	if cursor != nil {
		variables["cursor"] = *cursor
	}

	reqBody := graphQLRequest{
		Query:     reviewThreadsQuery,
		Variables: variables,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", graphQLURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute GraphQL query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("GraphQL query failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("failed to decode GraphQL response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, nil, fmt.Errorf("GraphQL errors: %v", result.Errors)
	}

	if result.Data == nil || result.Data.Repository == nil || result.Data.Repository.PullRequest == nil {
		return nil, nil, fmt.Errorf("unexpected GraphQL response structure")
	}

	graphQLThreads := result.Data.Repository.PullRequest.ReviewThreads
	if graphQLThreads == nil {
		return nil, nil, nil
	}

	// Convert GraphQL types to our types
	var threads []ReviewThread
	for _, t := range graphQLThreads.Nodes {
		thread := ReviewThread{
			ID:         t.ID,
			IsResolved: t.IsResolved,
			Path:       t.Path,
			Line:       t.Line,
		}

		if t.Comments != nil {
			for _, comment := range t.Comments.Nodes {
				author := ""
				if comment.Author != nil {
					author = comment.Author.Login
				}
				thread.Comments = append(thread.Comments, ThreadComment{
					ID:        comment.ID,
					Body:      comment.Body,
					Author:    author,
					CreatedAt: comment.CreatedAt,
				})
			}
		}

		threads = append(threads, thread)
	}

	return threads, graphQLThreads.PageInfo, nil
}
