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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// FlagIsComponentsV2 marks a message built with v2 components, which the app does not own.
const FlagIsComponentsV2 = 1 << 15

// WebhookPayload is the body of an Execute Webhook request.
// https://docs.discord.com/developers/resources/webhook#execute-webhook
type WebhookPayload struct {
	Content    string            `json:"content,omitempty"`
	Embeds     []json.RawMessage `json:"embeds,omitempty"`
	Components []json.RawMessage `json:"components,omitempty"`
	Flags      int               `json:"flags,omitempty"`
}

// SendWebhook posts payload to the Discord webhook configured for Marketplace
// notifications.
func (c *Client) SendWebhook(ctx context.Context, payload WebhookPayload) error {
	idToken := string(c.cfg.MarketplaceWebhook)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: encoding webhook payload: %w", err)
	}

	path := "/webhooks/" + idToken
	if len(payload.Components) > 0 {
		path += "?with_components=true"
	}

	req, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord: creating webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord: sending webhook: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return parseRateLimitError(resp, respBody, parseAPIError(resp.StatusCode, respBody))
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return parseAPIError(resp.StatusCode, respBody)
	}

	return nil
}
