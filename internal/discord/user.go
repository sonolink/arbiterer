/*
Copyright 2026-present SonoLink.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// User is the parsed view of the Discord user.
type User struct {
	ID  string          `json:"id"`
	Raw json.RawMessage `json:"-"`
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

	user.Raw = body

	return &user, nil
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
