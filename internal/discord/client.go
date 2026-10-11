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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sonolink/arbiterer/internal/config"
)

const maxResponseBytes = 1 << 20

// Client talks to the Discord API using the given application credentials.
type Client struct {
	cfg        config.Discord
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client from a Discord configuration.
func NewClient(cfg config.Discord) *Client {
	return &Client{
		cfg:        cfg,
		baseURL:    "https://discord.com/api/v10",
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) newRequest(
	ctx context.Context,
	method,
	path string,
	body io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	// version is a place holder, it should actually later come from somewhere standard.
	req.Header.Set("User-Agent", "arbiterer/0.1 (https://github.com/sonolink/arbiterer)")

	return req, nil
}

// readBody reads a response.
// If the response bytes exceed maxResponseBytes an error is returned.
func readBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}

	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("discord: response exceeds %d bytes", maxResponseBytes)
	}

	return body, nil
}
