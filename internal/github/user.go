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
