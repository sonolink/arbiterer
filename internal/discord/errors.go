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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// OAuthError is an error returned by Discord's OAuth token endpoint.
type OAuthError struct {
	Status      int
	Code        string
	Description string
}

func (e *OAuthError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf(
			"discord oauth: %s: %s (http %d)",
			e.Code,
			e.Description,
			e.Status,
		)
	}

	return fmt.Sprintf("discord oauth: %s (http %d)", e.Code, e.Status)
}

func parseOAuthError(status int, body []byte) error {
	var payload struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &payload)

	return &OAuthError{
		Status:      status,
		Code:        payload.Error,
		Description: payload.Description,
	}
}

// APIError is an error returned by a Discord REST endpoint.
type APIError struct {
	Status  int
	Code    int
	Message string
	Body    string
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf(
			"discord api: %s (http %d, code %d)",
			e.Message,
			e.Status,
			e.Code,
		)
	}

	return fmt.Sprintf("discord api: http %d: %s (body: %.200q)", e.Status, e.Message, e.Body)
}

func parseAPIError(status int, body []byte) error {
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &payload)

	msg := payload.Message
	if msg == "" {
		msg = http.StatusText(status)
	}

	return &APIError{
		Status:  status,
		Code:    payload.Code,
		Message: msg,
		Body:    string(body),
	}
}

// RateLimitError is an error returned by Discord when a ratelimit is hit on their end.
type RateLimitError struct {
	Err        error
	RetryAfter time.Duration
	Global     bool
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf(
		"discord: rate limited, retry after %s (global=%t): %v",
		e.RetryAfter,
		e.Global,
		e.Err,
	)
}

func (e *RateLimitError) Unwrap() error {
	return e.Err
}

func parseRateLimitError(resp *http.Response, body []byte, base error) error {
	var payload struct {
		Message    string  `json:"message"`
		RetryAfter float64 `json:"retry_after"`
		Global     bool    `json:"global"`
	}

	retryAfter := time.Duration(0)
	global := false

	if err := json.Unmarshal(body, &payload); err == nil && payload.RetryAfter > 0 {
		retryAfter = time.Duration(payload.RetryAfter * float64(time.Second))
		global = payload.Global
	} else if h := resp.Header.Get("Retry-After"); h != "" {
		if secs, perr := strconv.ParseFloat(h, 64); perr == nil {
			retryAfter = time.Duration(secs * float64(time.Second))
		}
	}

	if resp.Header.Get("X-RateLimit-Global") == "true" {
		global = true
	}

	return &RateLimitError{
		Err:        base,
		RetryAfter: retryAfter,
		Global:     global,
	}
}
