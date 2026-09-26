package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// AuthorizeURL builds the GitHub consent page URL for the given state.
func (c *Client) AuthorizeURL(state string) string {
	const oauthURL = "https://github.com"

	q := url.Values{
		"client_id":    {c.cfg.ClientID},
		"redirect_uri": {c.cfg.RedirectURI},
		"state":        {state},
	}
	u := url.URL{
		Path:     "/login/oauth/authorize",
		RawQuery: q.Encode(),
	}

	return oauthURL + u.RequestURI()
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ExchangeCode swaps an OAuth authorization code for an access token.
func (c *Client) ExchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.cfg.RedirectURI},
	}

	req, err := c.newOAuthRequest(
		ctx,
		http.MethodPost,
		"/login/oauth/access_token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("github: exchanging code: http %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("github: decoding token response: %w (body: %.200q)", err, body)
	}

	if tr.Error != "" {
		return "", fmt.Errorf("github: exchanging code: %s: %s", tr.Error, tr.ErrorDescription)
	}

	if tr.AccessToken == "" {
		return "", errors.New("github: token response has no access token")
	}

	return tr.AccessToken, nil
}
