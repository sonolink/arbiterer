package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

// appSlug caches the app's URL name, which never changes for a given app.
type appSlug struct {
	mu   sync.Mutex
	slug string
}

// slug returns the app's URL name, fetching it from GitHub on first use.
func (c *Client) slug(ctx context.Context) (string, error) {
	c.appSlug.mu.Lock()
	defer c.appSlug.mu.Unlock()

	if c.appSlug.slug != "" {
		return c.appSlug.slug, nil
	}

	appJWT, err := c.generateJWT()
	if err != nil {
		return "", err
	}

	var app struct {
		Slug string `json:"slug"`
	}
	if err := c.sendRequest(ctx, http.MethodGet, "/app", appJWT, nil, &app); err != nil {
		return "", fmt.Errorf("github: fetching app: %w", err)
	}

	c.appSlug.slug = app.Slug

	return app.Slug, nil
}

// InstallURL returns the page for installing the app on the given repository.
// The account and repository are preselected when GitHub honors the hints,
// and the page falls back to a plain install otherwise.
func (c *Client) InstallURL(ctx context.Context, ownerID, repositoryID int64) (string, error) {
	slug, err := c.slug(ctx)
	if err != nil {
		return "", err
	}

	u := url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   "/apps/" + url.PathEscape(slug) + "/installations/new",
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
