package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Token holds the result of a Discord OAuth exchange. ExpiresAt is computed locally
// from the seconds Discord reports.
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
}

// AuthorizeURL builds the Discord consent page URL for the given state and scopes.
func (c *Client) AuthorizeURL(state string, scopes ...string) (string, error) {
	if len(scopes) == 0 {
		return "", fmt.Errorf("discord: at least one scope is required")
	}

	q := url.Values{
		"client_id":     {c.cfg.ClientID},
		"response_type": {"code"},
		"redirect_uri":  {c.cfg.RedirectURI},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
		"prompt":        {"consent"},
	}

	u := url.URL{
		Scheme:   "https",
		Host:     "discord.com",
		Path:     "/oauth2/authorize",
		RawQuery: q.Encode(),
	}
	return u.String(), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func (tr tokenResponse) token() (*Token, error) {
	if !strings.EqualFold(tr.TokenType, "Bearer") {
		return nil, fmt.Errorf("discord: unexpected token_type %q", tr.TokenType)
	}

	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		Scopes:       strings.Fields(tr.Scope),
	}, nil
}

// Exchange swaps an authorization code for a token.
func (c *Client) Exchange(ctx context.Context, code string) (*Token, error) {
	return c.requestToken(
		ctx,
		url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {code},
			"redirect_uri": {c.cfg.RedirectURI},
		},
	)
}

// Refresh uses a refresh token to get a fresh access token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	return c.requestToken(
		ctx,
		url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {refreshToken},
		},
	)
}

func (c *Client) requestToken(ctx context.Context, form url.Values) (*Token, error) {
	req, err := c.newRequest(
		ctx,
		http.MethodPost,
		"/oauth2/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)

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
		return nil, parseRateLimitError(resp, body, parseOAuthError(resp.StatusCode, body))
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, parseOAuthError(resp.StatusCode, body)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("discord: decoding token response: %w (body: %.200q)", err, body)
	}

	return tr.token()
}
