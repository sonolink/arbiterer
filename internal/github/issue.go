package github

import (
	"context"
	"fmt"
	"net/http"
)

// FetchIssueAuthor returns the user who opened the given issue (pull request).
func (c *Client) FetchIssueAuthor(
	ctx context.Context,
	token string,
	repo string,
	issueNumber int64,
) (*User, error) {
	var issue struct {
		User User `json:"user"`
	}
	if err := c.sendRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/issues/%d", repo, issueNumber),
		token,
		nil,
		&issue,
	); err != nil {
		return nil, fmt.Errorf("github: fetching issue: %w", err)
	}

	return &issue.User, nil
}

// Comment is a comment on a pull request.
type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// CreateComment posts a new comment on the given issue (pull request) and returns its id.
func (c *Client) CreateComment(
	ctx context.Context,
	token string,
	repo string,
	issueNumber int64,
	body string,
) (int64, error) {
	var comment Comment
	if err := c.sendRequest(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/repos/%s/issues/%d/comments", repo, issueNumber),
		token,
		map[string]string{"body": body},
		&comment,
	); err != nil {
		return 0, fmt.Errorf("github: creating comment: %w", err)
	}

	return comment.ID, nil
}

// UpdateComment replaces the body of an existing comment.
func (c *Client) UpdateComment(ctx context.Context, token, repo string, commentID int64, body string) error {
	if err := c.sendRequest(
		ctx,
		http.MethodPatch,
		fmt.Sprintf("/repos/%s/issues/comments/%d", repo, commentID),
		token,
		map[string]string{"body": body},
		nil,
	); err != nil {
		return fmt.Errorf("github: updating comment: %w", err)
	}

	return nil
}
