package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sonolink/arbiterer/internal/storage"
)

const (
	// linkTokenAAD is the AAD context for link tokens.
	linkTokenAAD = "arbiterer/link-token/v1"

	// linkCookieAAD is the AAD context for the linking cookie.
	linkCookieAAD = "arbiterer/link-cookie/v1"

	// linkCookieName is the name of the linking cookie.
	linkCookieName = "arbiterer_link"

	// linkDiscordScopes are the OAuth scopes requested from Discord.
	linkDiscordScopes = "identify guilds.members.read"
)

const (
	linkDetailInvalidOrExpired = "This link is invalid or has expired. Please re-run the check to generate a fresh one."
	linkDetailExpired          = "This link has expired. Please re-run the check to get a fresh one."
	linkDetailRestart          = "This linking attempt has expired or was not started in this browser. Please re-run the check to start again."
	linkDetailGitHubAuth       = "GitHub authorization failed. Please try again."
	linkDetailGitHubUser       = "Could not read your GitHub account. Please try again."
	linkDetailIdentityMismatch = "This link belongs to a different GitHub account."
	linkDetailDiscordAuth      = "Discord authorization failed. Please try again."
	linkDetailDiscordUser      = "Could not read your Discord account. Please try again."
	linkDetailInternal         = "Something went wrong. Please try again."
)

var errLinkExpired = errors.New("link token expired")

// linkToken is the sealed payload carried through the URL from /v1/resolve.
type linkToken struct {
	GitHubUserID      string    `json:"github_user_id"`
	RepositoryID      int64     `json:"repository_id"`
	Repository        string    `json:"repository"`
	PullRequestNumber int64     `json:"pull_request_number"`
	Expiry            time.Time `json:"expiry"`
}

// linkCookie is the sealed payload carried between the GitHub and Discord.
type linkCookie struct {
	Nonce             string `json:"nonce"`
	RepositoryID      int64  `json:"repository_id"`
	GitHubUserID      string `json:"github_user_id"`
	Repository        string `json:"repository"`
	PullRequestNumber int64  `json:"pull_request_number"`
}

// sealLinkToken produces a URL-safe bearer token for a resolving link.
func (s *Server) sealLinkToken(githubUserID string, repositoryID int64, repo string, pullRequestNumber int64) (string, error) {
	payload, err := json.Marshal(linkToken{
		GitHubUserID:      githubUserID,
		RepositoryID:      repositoryID,
		Repository:        repo,
		PullRequestNumber: pullRequestNumber,
		Expiry:            time.Now().Add(s.cfg.LinkTokenLifetime),
	})
	if err != nil {
		return "", fmt.Errorf("sealing link token: %w", err)
	}

	sealed, err := s.tokenSealer.Seal(payload, []byte(linkTokenAAD))
	if err != nil {
		return "", fmt.Errorf("sealing link token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// openLinkToken decodes, unseals and expiry-checks a link token.
func (s *Server) openLinkToken(encoded string) (*linkToken, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding link token: %w", err)
	}

	payload, err := s.tokenSealer.Open(sealed, []byte(linkTokenAAD))
	if err != nil {
		return nil, fmt.Errorf("opening link token: %w", err)
	}

	var lt linkToken
	if err := json.Unmarshal(payload, &lt); err != nil {
		return nil, fmt.Errorf("decoding link token: %w", err)
	}

	if time.Now().After(lt.Expiry) {
		return nil, errLinkExpired
	}

	return &lt, nil
}

// sealLinkCookie seals the handoff between the GitHub and Discord.
func (s *Server) sealLinkCookie(c linkCookie) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("sealing link cookie: %w", err)
	}

	sealed, err := s.cookieSealer.Seal(payload, []byte(linkCookieAAD))
	if err != nil {
		return "", fmt.Errorf("sealing link cookie: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// openLinkCookie unseals a value produced by sealLinkCookie.
func (s *Server) openLinkCookie(encoded string) (*linkCookie, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding link cookie: %w", err)
	}

	payload, err := s.cookieSealer.Open(sealed, []byte(linkCookieAAD))
	if err != nil {
		return nil, fmt.Errorf("opening link cookie: %w", err)
	}

	var c linkCookie
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("decoding link cookie: %w", err)
	}

	return &c, nil
}

// newNonce returns a random, URL-safe value used as the OAuth state nonce.
func newNonce() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// writeLinkCookie sets or clears the linking cookie. A negative maxAge expires it.
func (s *Server) writeLinkCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     linkCookieName,
		Value:    value,
		Path:     "/link",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// --- GET /link?token=... ---.
func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	encoded := r.URL.Query().Get("token")
	if encoded == "" {
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailInvalidOrExpired)
		return
	}

	lt, err := s.openLinkToken(encoded)
	if errors.Is(err, errLinkExpired) {
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailExpired)
		return
	}
	if err != nil {
		s.logger.Warn("rejecting link token", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailInvalidOrExpired)
		return
	}

	state, err := s.sealLinkToken(lt.GitHubUserID, lt.RepositoryID, lt.Repository, lt.PullRequestNumber)
	if err != nil {
		s.logger.Error("sealing link state", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	http.Redirect(w, r, s.githubClient.AuthorizeURL(state), http.StatusFound)
}

// --- GET /link/github/callback?code=...&state=... ---.
func (s *Server) handleLinkGitHubCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	state := r.URL.Query().Get("state")
	if state == "" {
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailInvalidOrExpired)
		return
	}

	lt, err := s.openLinkToken(state)
	if err != nil {
		s.logger.Warn("rejecting github callback state", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailInvalidOrExpired)
		return
	}

	code := r.URL.Query().Get("code")

	accessToken, err := s.githubClient.Exchange(ctx, code)
	if err != nil {
		s.logger.Error("github oauth exchange failed", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailGitHubAuth)
		return
	}

	ghUser, err := s.githubClient.User(ctx, accessToken)
	if err != nil {
		s.logger.Error("fetching github user failed", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailGitHubUser)
		return
	}

	if strconv.FormatInt(ghUser.ID, 10) != lt.GitHubUserID {
		s.logger.Warn("github identity mismatch",
			"expected", lt.GitHubUserID,
			"got", ghUser.ID,
		)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailIdentityMismatch)
		return
	}

	nonce, err := newNonce()
	if err != nil {
		s.logger.Error("generating nonce", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	sealedCookie, err := s.sealLinkCookie(linkCookie{
		Nonce:             nonce,
		RepositoryID:      lt.RepositoryID,
		GitHubUserID:      lt.GitHubUserID,
		Repository:        lt.Repository,
		PullRequestNumber: lt.PullRequestNumber,
	})
	if err != nil {
		s.logger.Error("sealing link cookie", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	s.writeLinkCookie(w, sealedCookie, int(s.cfg.LinkCookieLifetime.Seconds()))

	discordURL, err := s.discordClient.AuthorizeURL(nonce, linkDiscordScopes)
	if err != nil {
		s.logger.Error("building discord auth url", "error", err)
		s.writeLinkCookie(w, "", -1)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	http.Redirect(w, r, discordURL, http.StatusFound)
}

// --- GET /link/discord/callback?code=...&state=... ---.
func (s *Server) handleLinkDiscordCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cookie, err := r.Cookie(linkCookieName)
	if err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	lc, err := s.openLinkCookie(cookie.Value)
	if err != nil {
		s.logger.Warn("rejecting link cookie", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	state := r.URL.Query().Get("state")
	if subtle.ConstantTimeCompare([]byte(state), []byte(lc.Nonce)) != 1 {
		s.logger.Warn("link cookie nonce mismatch")
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	code := r.URL.Query().Get("code")
	token, err := s.discordClient.Exchange(ctx, code)
	if err != nil {
		s.logger.Error("discord ouath exchange failed", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailDiscordAuth)
		return
	}

	du, err := s.discordClient.Me(ctx, token.AccessToken)
	if err != nil {
		s.logger.Error("fetching discord user failed", "error", err)
		s.writeProblem(w, r, http.StatusBadGateway, linkDetailDiscordUser)
		return
	}

	discordUserID, err := strconv.ParseInt(du.ID, 10, 64)
	if err != nil {
		s.logger.Error("parsing discord user id", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	sealedAccess, err := s.tokenSealer.Seal(
		[]byte(token.AccessToken),
		tokenAAD(discordUserID, aadFieldAccess),
	)
	if err != nil {
		s.logger.Error("sealing discord access token", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	sealedRefresh, err := s.tokenSealer.Seal(
		[]byte(token.RefreshToken),
		tokenAAD(discordUserID, aadFieldRefresh),
	)
	if err != nil {
		s.logger.Error("sealing discord refresh token", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	linkUser := &storage.DiscordUser{
		ID:                    discordUserID,
		EncryptedAccessToken:  sealedAccess,
		EncryptedRefreshToken: sealedRefresh,
		TokenExpiresAt:        token.ExpiresAt,
	}

	if err := s.store.LinkGitHubDiscord(
		ctx,
		lc.GitHubUserID,
		lc.RepositoryID,
		linkUser,
	); err != nil {
		s.logger.Error("persisting link", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	s.writeLinkCookie(w, "", -1)

	if lc.PullRequestNumber != 0 {
		if err := s.syncSetupComment(ctx, lc.Repository, lc.PullRequestNumber, statusLinked, "", false); err != nil {
			s.logger.Error("syncing setup comment", "error", err)
		}
	}

	s.writeJSON(w, http.StatusOK, "Your GitHub and Discord accounts are now connected.")
}
