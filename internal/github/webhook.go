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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// webhookSignaturePrefix marks the HMAC algorithm GitHub used for a signature.
const webhookSignaturePrefix = "sha256="

// VerifyWebhook reports whether signature authenticates payload with the app's
// webhook secret. GitHub signs the raw request body with HMAC-SHA256.
// https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
func (c *Client) VerifyWebhook(payload []byte, signature string) bool {
	if c.cfg.WebhookSecret == "" {
		return false
	}

	encoded, ok := strings.CutPrefix(signature, webhookSignaturePrefix)
	if !ok {
		return false
	}

	got, err := hex.DecodeString(encoded)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(c.cfg.WebhookSecret))
	mac.Write(payload)

	return hmac.Equal(mac.Sum(nil), got)
}
