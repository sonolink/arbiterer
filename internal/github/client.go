package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sonolink/arbiterer/internal/config"
)

const (
	maxResponseBytes = 1 << 20

	// version is a place holder, it should actually later come from somewhere standard.
	userAgent = "arbiterer/0.1 (https://github.com/sonolink/arbiterer)"
)

// Client talks to GitHub using the given application credentials.
type Client struct {
	cfg        config.GitHub
	httpClient *http.Client
}

// NewClient builds a Client from a GitHub configuration.
func NewClient(cfg config.GitHub) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) newAPIRequest(
	ctx context.Context,
	method,
	path string,
	body io.Reader,
) (*http.Request, error) {
	const baseURL = "https://api.github.com"

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")

	return req, nil
}

func (c *Client) newOAuthRequest(
	ctx context.Context,
	method,
	path string,
	body io.Reader,
) (*http.Request, error) {
	const baseURL = "https://github.com"

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

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
		return nil, fmt.Errorf("github: response exceeds %d bytes", maxResponseBytes)
	}

	return body, nil
}
