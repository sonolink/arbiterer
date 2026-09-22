package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const commentsPerPage = 100

// PullRequestAuthorLogin returns the GitHub login of the user who opened the given pull request.
func (c *Client) PullRequestAuthorLogin(ctx context.Context, repo string, pullRequestNumber int64) (string, error) {
	var issue struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := c.doInstallationJSON(
		ctx,
		http.MethodGet,
		repo,
		fmt.Sprintf("/repos/%s/issues/%d", repo, pullRequestNumber),
		nil,
		&issue,
	); err != nil {
		return "", fmt.Errorf("github: fetching issue: %w", err)
	}

	return issue.User.Login, nil
}

// Comment is a comment on an issue or pull request.
type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// listComments returns every comment on the given issue or pull request.
func (c *Client) listComments(ctx context.Context, repo string, pullRequestNumber int64) ([]Comment, error) {
	var all []Comment

	for page := 1; ; page++ {
		var pageComments []Comment
		path := fmt.Sprintf(
			"/repos/%s/issues/%d/comments?per_page=%d&page=%d",
			repo, pullRequestNumber, commentsPerPage, page,
		)
		if err := c.doInstallationJSON(ctx, http.MethodGet, repo, path, nil, &pageComments); err != nil {
			return nil, err
		}

		all = append(all, pageComments...)

		if len(pageComments) < commentsPerPage {
			return all, nil
		}
	}
}

// FindComment returns the first comment on the given issue or pull request
// whose body contains marker, or nil if there is none.
func (c *Client) FindComment(ctx context.Context, repo string, pullRequestNumber int64, marker string) (*Comment, error) {
	comments, err := c.listComments(ctx, repo, pullRequestNumber)
	if err != nil {
		return nil, fmt.Errorf("github: listing comments: %w", err)
	}

	for _, c := range comments {
		if strings.Contains(c.Body, marker) {
			return &c, nil
		}
	}

	return nil, nil
}

// CreateComment posts a new comment on the given issue or pull request.
func (c *Client) CreateComment(ctx context.Context, repo string, pullRequestNumber int64, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, pullRequestNumber)

	return c.doInstallationJSON(ctx, http.MethodPost, repo, path, map[string]string{"body": body}, nil)
}

// UpdateComment replaces the body of an existing comment.
func (c *Client) UpdateComment(ctx context.Context, repo string, commentID int64, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/comments/%d", repo, commentID)

	return c.doInstallationJSON(ctx, http.MethodPatch, repo, path, map[string]string{"body": body}, nil)
}
