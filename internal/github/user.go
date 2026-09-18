package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// User is a minimal view of a GitHub user, holding just the id.
type User struct {
	ID int64 `json:"id"`
}

// User returns the data of the user behind the given access token.
func (c *Client) User(ctx context.Context, accessToken string) (*User, error) {
	req, err := c.newAPIRequest(ctx, http.MethodGet, "/user", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf(
			"github: fetching user: http %d (body: %.200q)",
			resp.StatusCode,
			body,
		)
	}

	var user User

	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("github: decoding user: %w (body: %.200q)", err, body)
	}

	if user.ID == 0 {
		return nil, errors.New("github: user response has no id")
	}

	return &user, nil
}
