package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// User is a minimal view of a Discord user, holding just the id.
type User struct {
	ID string `json:"id"`
}

// Me returns the data of the user behind the given access token.
func (c *Client) Me(ctx context.Context, accessToken string) (*User, error) {
	body, err := c.get(ctx, accessToken, "/users/@me", "user")
	if err != nil {
		return nil, err
	}

	var user User
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("discord: decoding user: %w (body: %.200q)", err, body)
	}

	return &user, nil
}

// MeRaw returns the user behind the given access token as raw JSON.
func (c *Client) MeRaw(ctx context.Context, accessToken string) (json.RawMessage, error) {
	return c.get(ctx, accessToken, "/users/@me", "user")
}

// GuildMember returns the member record as JSON. It stays raw so callers can
// pick the fields they need without this package guessing the schema.
func (c *Client) GuildMember(ctx context.Context, accessToken, guildID string) (json.RawMessage, error) {
	return c.get(
		ctx,
		accessToken,
		"/users/@me/guilds/"+url.PathEscape(guildID)+"/member",
		"member",
	)
}

// get reads a JSON endpoint, handling status codes and rejecting a body that will not decode.
func (c *Client) get(
	ctx context.Context,
	accessToken,
	path,
	what string,
) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
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

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, parseRateLimitError(resp, body, parseAPIError(resp.StatusCode, body))
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, parseAPIError(resp.StatusCode, body)
	}

	if !json.Valid(body) {
		return nil, fmt.Errorf("discord: %s response is not valid JSON (body: %.200q)", what, body)
	}

	return body, nil
}
