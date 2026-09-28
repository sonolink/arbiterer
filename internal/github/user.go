package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// User is a minimal view of a GitHub user.
type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// FetchUser returns the user behind the given OAuth access token.
func (c *Client) FetchUser(ctx context.Context, accessToken string) (*User, error) {
	var user User
	if err := c.sendRequest(ctx, http.MethodGet, "/user", accessToken, nil, &user); err != nil {
		return nil, fmt.Errorf("github: fetching user: %w", err)
	}

	if user.ID == 0 {
		return nil, errors.New("github: user response has no id")
	}

	return &user, nil
}
