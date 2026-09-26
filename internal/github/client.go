package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sonolink/arbiterer/internal/config"
)

const (
	maxResponseBytes = 1 << 20

	// version is a place holder, it should actually later come from somewhere standard.
	userAgent = "arbiterer/0.1 (https://github.com/sonolink/arbiterer)"

	// jwtLifetime is how long a GitHub App JWT is valid for (maximum of 10 minutes).
	jwtLifetime = 9 * time.Minute

	// jwtClockSkew backdates the issued-at claim so a slightly-behind clock on
	// GitHub's side still accepts the token.
	jwtClockSkew = 60 * time.Second
)

// Client talks to GitHub using the given application credentials.
type Client struct {
	cfg        config.GitHub
	appSlug    appSlug
	httpClient *http.Client
}

// NewClient builds a Client from a GitHub configuration.
func NewClient(cfg config.GitHub) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// generateJWT signs a short-lived JWT identifying the app itself.
func (c *Client) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    c.cfg.ClientID,
		IssuedAt:  jwt.NewNumericDate(now.Add(-jwtClockSkew)),
		ExpiresAt: jwt.NewNumericDate(now.Add(jwtLifetime)),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).
		SignedString((*rsa.PrivateKey)(&c.cfg.PrivateKey))
	if err != nil {
		return "", fmt.Errorf("github: signing app jwt: %w", err)
	}

	return signed, nil
}

// CreateInstallationToken creates an installation token that can only act on
// the given repository.
func (c *Client) CreateInstallationToken(ctx context.Context, repositoryID int64, repo string) (string, error) {
	appJWT, err := c.generateJWT()
	if err != nil {
		return "", err
	}

	var installation struct {
		ID int64 `json:"id"`
	}
	if err := c.sendRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/installation", repo),
		appJWT,
		nil,
		&installation,
	); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return "", fmt.Errorf("%w: %s", ErrAppNotInstalled, repo)
		}

		return "", fmt.Errorf("github: fetching installation: %w", err)
	}

	var access struct {
		Token string `json:"token"`
	}
	if err := c.sendRequest(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/app/installations/%d/access_tokens", installation.ID),
		appJWT,
		map[string][]int64{"repository_ids": {repositoryID}},
		&access,
	); err != nil {
		return "", fmt.Errorf("github: creating installation token: %w", err)
	}

	return access.Token, nil
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

// sendRequest sends a request to the GitHub REST API authorized with token.
func (c *Client) sendRequest(ctx context.Context, method, path, token string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("github: encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := c.newAPIRequest(ctx, method, path, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &payload)

		msg := payload.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}

		return &APIError{Status: resp.StatusCode, Message: msg, Body: string(respBody)}
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("github: decoding response: %w (body: %.200q)", err, respBody)
	}

	return nil
}
