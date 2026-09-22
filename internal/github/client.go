package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sonolink/arbiterer/internal/config"
)

const (
	maxResponseBytes = 1 << 20

	// version is a place holder, it should actually later come from somewhere standard.
	userAgent = "arbiterer/0.1 (https://github.com/sonolink/arbiterer)"

	// jwtLifetime is how long a GitHub App JWT is valid for. GitHub allows at
	// most 10 minutes; this stays under that to absorb clock drift.
	jwtLifetime = 9 * time.Minute
	// jwtClockSkew backdates the issued-at claim so a slightly-behind clock on
	// GitHub's side still accepts the token, per GitHub's recommendation:
	// https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app
	jwtClockSkew = 60 * time.Second
	// installTokenSkew treats a cached installation token as expired this
	// long before it actually is, so a request never races the real expiry.
	installTokenSkew = time.Minute
)

// Client talks to GitHub using the given application credentials: it drives
// the OAuth login flow for linking a contributor's account, and authenticates
// as the Arbiterer GitHub App to act on repositories where it is installed.
type Client struct {
	cfg        config.GitHub
	httpClient *http.Client

	mu     sync.Mutex
	tokens map[string]cachedToken
}

// cachedToken is an installation access token, valid until expiresAt.
type cachedToken struct {
	token     string
	expiresAt time.Time
}

// NewClient builds a Client from a GitHub configuration.
func NewClient(cfg config.GitHub) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		tokens:     make(map[string]cachedToken),
	}
}

// appJWT signs a short-lived JWT identifying the app itself, used only to
// look up installations and mint installation tokens.
func (c *Client) appJWT() (string, error) {
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

// installationToken returns a token the app can use to act on repo ("owner/repo"),
// minting and caching a fresh one when none is cached or it is close to expiring.
func (c *Client) installationToken(ctx context.Context, repo string) (string, error) {
	c.mu.Lock()
	cached, ok := c.tokens[repo]
	c.mu.Unlock()

	if ok && time.Until(cached.expiresAt) > installTokenSkew {
		return cached.token, nil
	}

	appJWT, err := c.appJWT()
	if err != nil {
		return "", err
	}

	var installation struct {
		ID int64 `json:"id"`
	}
	if err := c.doJSON(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/installation", repo),
		appJWT,
		nil,
		&installation,
	); err != nil {
		return "", fmt.Errorf("github: finding installation: %w", err)
	}

	var access struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := c.doJSON(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/app/installations/%d/access_tokens", installation.ID),
		appJWT,
		nil,
		&access,
	); err != nil {
		return "", fmt.Errorf("github: minting installation token: %w", err)
	}

	c.mu.Lock()
	c.tokens[repo] = cachedToken{token: access.Token, expiresAt: access.ExpiresAt}
	c.mu.Unlock()

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

// doInstallationJSON sends a JSON request to the GitHub REST API, authorized
// as the app's installation on repo, and decodes a JSON response into out,
// if given.
func (c *Client) doInstallationJSON(ctx context.Context, method, repo, path string, body, out any) error {
	token, err := c.installationToken(ctx, repo)
	if err != nil {
		return err
	}

	return c.doJSON(ctx, method, path, token, body, out)
}

// doJSON sends a JSON request, authorized with token, to the GitHub REST API
// and decodes a JSON response into out, if given.
func (c *Client) doJSON(ctx context.Context, method, path, token string, body, out any) error {
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
