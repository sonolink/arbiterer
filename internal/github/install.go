package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// App is a minimal view of the GitHub App itself.
type App struct {
	Slug string `json:"slug"`
}

// fetchApp returns the app behind the configured credentials, fetching it
// from GitHub on first use. The app never changes, so the result is cached.
func (c *Client) fetchApp(ctx context.Context) (*App, error) {
	c.appMu.Lock()
	defer c.appMu.Unlock()

	if c.app != nil {
		return c.app, nil
	}

	appJWT, err := c.generateJWT()
	if err != nil {
		return nil, err
	}

	var app App
	if err := c.sendRequest(ctx, http.MethodGet, "/app", appJWT, nil, &app); err != nil {
		return nil, fmt.Errorf("github: fetching app: %w", err)
	}

	c.app = &app

	return c.app, nil
}

// InstallURL returns the page for installing the app on the given repository.
// The account and repository are preselected when GitHub honors the hints,
// and the page falls back to a plain install otherwise.
func (c *Client) InstallURL(ctx context.Context, ownerID, repositoryID int64) (string, error) {
	app, err := c.fetchApp(ctx)
	if err != nil {
		return "", err
	}

	u := url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   "/apps/" + url.PathEscape(app.Slug) + "/installations/new",
	}

	if ownerID != 0 {
		u.Path += "/permissions"
		u.RawQuery = url.Values{
			"suggested_target_id": {strconv.FormatInt(ownerID, 10)},
			"repository_ids[]":    {strconv.FormatInt(repositoryID, 10)},
		}.Encode()
	}

	return u.String(), nil
}
