package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sonolink/arbiterer/internal/config"
)

const (
	appBaseURL          = "https://api.github.com"
	maxAppResponseBytes = 1 << 20
	commentsPerPage     = 100

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

// AppClient authenticates as the Arbiterer GitHub App to act on repositories
// where it is installed.
type AppClient struct {
	clientID   string
	privateKey *rsa.PrivateKey
	httpClient *http.Client

	mu     sync.Mutex
	tokens map[string]cachedToken
}

// cachedToken is an installation access token, valid until expiresAt.
type cachedToken struct {
	token     string
	expiresAt time.Time
}

// NewAppClient builds an AppClient from the app's configuration.
func NewAppClient(cfg config.GitHub) *AppClient {
	return &AppClient{
		clientID:   cfg.ClientID,
		privateKey: (*rsa.PrivateKey)(&cfg.PrivateKey),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		tokens:     make(map[string]cachedToken),
	}
}

// appJWT signs a short-lived JWT identifying the app itself, used only to
// look up installations and mint installation tokens.
func (a *AppClient) appJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    a.clientID,
		IssuedAt:  jwt.NewNumericDate(now.Add(-jwtClockSkew)),
		ExpiresAt: jwt.NewNumericDate(now.Add(jwtLifetime)),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(a.privateKey)
	if err != nil {
		return "", fmt.Errorf("github: signing app jwt: %w", err)
	}

	return signed, nil
}

// installationToken returns a token the app can use to act on repo ("owner/repo"),
// minting and caching a fresh one when none is cached or it is close to expiring.
func (a *AppClient) installationToken(ctx context.Context, repo string) (string, error) {
	a.mu.Lock()
	cached, ok := a.tokens[repo]
	a.mu.Unlock()

	if ok && time.Until(cached.expiresAt) > installTokenSkew {
		return cached.token, nil
	}

	appJWT, err := a.appJWT()
	if err != nil {
		return "", err
	}

	var installation struct {
		ID int64 `json:"id"`
	}
	if err := a.doJSON(
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
	if err := a.doJSON(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/app/installations/%d/access_tokens", installation.ID),
		appJWT,
		nil,
		&access,
	); err != nil {
		return "", fmt.Errorf("github: minting installation token: %w", err)
	}

	a.mu.Lock()
	a.tokens[repo] = cachedToken{token: access.Token, expiresAt: access.ExpiresAt}
	a.mu.Unlock()

	return access.Token, nil
}

// IssueAuthorLogin returns the GitHub login of the user who opened the given
// issue or pull request.
func (a *AppClient) IssueAuthorLogin(ctx context.Context, repo string, issueNumber int64) (string, error) {
	token, err := a.installationToken(ctx, repo)
	if err != nil {
		return "", err
	}

	var issue struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := a.doJSON(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/issues/%d", repo, issueNumber),
		token,
		nil,
		&issue,
	); err != nil {
		return "", fmt.Errorf("github: fetching issue: %w", err)
	}

	return issue.User.Login, nil
}

// Comment is a comment on an issue or pull request.
type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// listComments returns every comment on the given issue or pull request.
func (a *AppClient) listComments(ctx context.Context, repo string, issueNumber int64, token string) ([]Comment, error) {
	var all []Comment

	for page := 1; ; page++ {
		var pageComments []Comment
		path := fmt.Sprintf(
			"/repos/%s/issues/%d/comments?per_page=%d&page=%d",
			repo, issueNumber, commentsPerPage, page,
		)
		if err := a.doJSON(ctx, http.MethodGet, path, token, nil, &pageComments); err != nil {
			return nil, err
		}

		all = append(all, pageComments...)

		if len(pageComments) < commentsPerPage {
			return all, nil
		}
	}
}

// FindComment returns the first comment on the given issue or pull request
// whose body contains marker, or nil if there is none.
func (a *AppClient) FindComment(ctx context.Context, repo string, issueNumber int64, marker string) (*Comment, error) {
	token, err := a.installationToken(ctx, repo)
	if err != nil {
		return nil, err
	}

	comments, err := a.listComments(ctx, repo, issueNumber, token)
	if err != nil {
		return nil, fmt.Errorf("github: listing comments: %w", err)
	}

	for _, c := range comments {
		if strings.Contains(c.Body, marker) {
			return &c, nil
		}
	}

	return nil, nil
}

// CreateComment posts a new comment on the given issue or pull request.
func (a *AppClient) CreateComment(ctx context.Context, repo string, issueNumber int64, body string) error {
	token, err := a.installationToken(ctx, repo)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, issueNumber)

	return a.doJSON(ctx, http.MethodPost, path, token, map[string]string{"body": body}, nil)
}

// UpdateComment replaces the body of an existing comment.
func (a *AppClient) UpdateComment(ctx context.Context, repo string, commentID int64, body string) error {
	token, err := a.installationToken(ctx, repo)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/repos/%s/issues/comments/%d", repo, commentID)

	return a.doJSON(ctx, http.MethodPatch, path, token, map[string]string{"body": body}, nil)
}

// doJSON sends a JSON request to the GitHub API and decodes a JSON response
// into out, if given.
func (a *AppClient) doJSON(ctx context.Context, method, path, token string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("github: encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, appBaseURL+path, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "arbiterer/0.1 (https://github.com/sonolink/arbiterer)")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxAppResponseBytes+1))
	if err != nil {
		return err
	}

	if len(respBody) > maxAppResponseBytes {
		return fmt.Errorf("github: response exceeds %d bytes", maxAppResponseBytes)
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
