package github

import (
	"context"
	"fmt"
	"net/http"
)

// FetchPullRequestAuthorLogin returns the GitHub login of the user who opened
// the given pull request.
func (c *Client) FetchPullRequestAuthorLogin(
	ctx context.Context,
	token string,
	repo string,
	pullRequestNumber int64,
) (string, error) {
	var issue struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := c.sendRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/issues/%d", repo, pullRequestNumber),
		token,
		nil,
		&issue,
	); err != nil {
		return "", fmt.Errorf("github: fetching pull request: %w", err)
	}

	return issue.User.Login, nil
}

// Comment is a comment on a pull request.
type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// CreateComment posts a new comment on the given pull request and returns its id.
func (c *Client) CreateComment(
	ctx context.Context,
	token string,
	repo string,
	pullRequestNumber int64,
	body string,
) (int64, error) {
	var comment Comment
	if err := c.sendRequest(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/repos/%s/issues/%d/comments", repo, pullRequestNumber),
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
