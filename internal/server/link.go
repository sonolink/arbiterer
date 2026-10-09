package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sonolink/arbiterer/internal/storage"
)

// AAD contexts. Each sealed payload type has its own.
const (
	linkTokenAAD       = "arbiterer/link-token/v1"
	linkStateAAD       = "arbiterer/link-state/v1"
	linkCookieAAD      = "arbiterer/link-cookie/v1"
	linkStateCookieAAD = "arbiterer/link-state-cookie/v1"
)

const (
	// linkCookieName is the name of the handoff cookie. It is written only
	// after the GitHub identity check, so holding it means GitHub is verified.
	linkCookieName = "arbiterer_link"

	// linkStateCookieName is the name of the cookie carrying the browser binding
	// for the GitHub step.
	linkStateCookieName = "arbiterer_link_state"

	// linkPath is the path the linking flow is served under, and the scope both
	// cookies are limited to.
	linkPath = "/link"

	// linkDiscordScopes are the OAuth scopes requested from Discord.
	linkDiscordScopes = "identify guilds.members.read"
)

const (
	linkDetailRestart          = "This linking attempt is no longer valid. Please re-run the check to start again."
	linkDetailGitHubAuth       = "GitHub authorization failed. Please try again."
	linkDetailGitHubUser       = "Could not read your GitHub account. Please try again."
	linkDetailIdentityMismatch = "This link belongs to a different GitHub account."
	linkDetailDiscordAuth      = "Discord authorization failed. Please try again."
	linkDetailDiscordTaken     = "This Discord account is already linked to another GitHub account for this repository."
	linkDetailDiscordUser      = "Could not read your Discord account. Please try again."
	linkDetailInternal         = "Something went wrong. Please try again."
)

// linkExpiredf reports which part of the linking flow expired.
func linkExpiredf(part string) error {
	return fmt.Errorf("link %s expired", part)
}

// linkToken is the sealed payload carried through the URL from /v1/discord/resolve.
type linkToken struct {
	GitHubUserID      string    `json:"github_user_id"`
	RepositoryID      int64     `json:"repository_id"`
	Repository        string    `json:"repository"`
	IssueNumber       int64     `json:"issue_number"`
	RunID             int64     `json:"run_id,omitempty"`
	Nonce             string    `json:"nonce,omitempty"`
	Expiry            time.Time `json:"expiry"`
	SkipLinkedComment bool      `json:"skip_linked_comment,omitempty"`
}

// linkCookie is the sealed handoff from the GitHub step to the Discord step.
type linkCookie struct {
	Nonce             string    `json:"nonce"`
	RepositoryID      int64     `json:"repository_id"`
	GitHubUserID      string    `json:"github_user_id"`
	Repository        string    `json:"repository"`
	IssueNumber       int64     `json:"issue_number"`
	RunID             int64     `json:"run_id,omitempty"`
	Expiry            time.Time `json:"expiry"`
	SkipLinkedComment bool      `json:"skip_linked_comment,omitempty"`
}

// linkStateCookie is the sealed browser binding for the GitHub step.
type linkStateCookie struct {
	Nonce  string    `json:"nonce"`
	Expiry time.Time `json:"expiry"`
}

// sealLinkToken produces a URL-safe bearer token sealed under aad, expiring
// after the configured lifetime.
func (s *Server) sealLinkToken(lt linkToken, aad string) (string, error) {
	lt.Expiry = time.Now().Add(s.cfg.LinkTokenLifetime)

	payload, err := json.Marshal(lt)
	if err != nil {
		return "", fmt.Errorf("sealing link token: %w", err)
	}

	sealed, err := s.browserSealer.Seal(payload, []byte(aad))
	if err != nil {
		return "", fmt.Errorf("sealing link token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// openLinkToken decodes, unseals and expiry-checks a token sealed under aad.
func (s *Server) openLinkToken(encoded, aad string) (linkToken, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return linkToken{}, fmt.Errorf("decoding link token: %w", err)
	}

	payload, err := s.browserSealer.Open(sealed, []byte(aad))
	if err != nil {
		return linkToken{}, fmt.Errorf("opening link token: %w", err)
	}

	var lt linkToken
	if err := json.Unmarshal(payload, &lt); err != nil {
		return linkToken{}, fmt.Errorf("decoding link token: %w", err)
	}

	if time.Now().After(lt.Expiry) {
		return linkToken{}, linkExpiredf("token")
	}

	return lt, nil
}

// sealLinkCookie seals the handoff between the GitHub and Discord steps,
// expiring after the configured lifetime.
func (s *Server) sealLinkCookie(c linkCookie) (string, error) {
	c.Expiry = time.Now().Add(s.cfg.LinkCookieLifetime)

	payload, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("sealing link cookie: %w", err)
	}

	sealed, err := s.browserSealer.Seal(payload, []byte(linkCookieAAD))
	if err != nil {
		return "", fmt.Errorf("sealing link cookie: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// openLinkCookie unseals a value produced by sealLinkCookie.
func (s *Server) openLinkCookie(encoded string) (linkCookie, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return linkCookie{}, fmt.Errorf("decoding link cookie: %w", err)
	}

	payload, err := s.browserSealer.Open(sealed, []byte(linkCookieAAD))
	if err != nil {
		return linkCookie{}, fmt.Errorf("opening link cookie: %w", err)
	}

	var c linkCookie
	if err := json.Unmarshal(payload, &c); err != nil {
		return linkCookie{}, fmt.Errorf("decoding link cookie: %w", err)
	}

	if time.Now().After(c.Expiry) {
		return linkCookie{}, linkExpiredf("cookie")
	}

	return c, nil
}

// sealLinkStateCookie seals the browser binding, expiring after the configured lifetime.
func (s *Server) sealLinkStateCookie(nonce string) (string, error) {
	payload, err := json.Marshal(linkStateCookie{
		Nonce:  nonce,
		Expiry: time.Now().Add(s.cfg.LinkCookieLifetime),
	})
	if err != nil {
		return "", fmt.Errorf("sealing link state cookie: %w", err)
	}

	sealed, err := s.browserSealer.Seal(payload, []byte(linkStateCookieAAD))
	if err != nil {
		return "", fmt.Errorf("sealing link state cookie: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// openLinkStateCookie unseals a value produced by sealLinkStateCookie.
func (s *Server) openLinkStateCookie(encoded string) (linkStateCookie, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return linkStateCookie{}, fmt.Errorf("decoding link state cookie: %w", err)
	}

	payload, err := s.browserSealer.Open(sealed, []byte(linkStateCookieAAD))
	if err != nil {
		return linkStateCookie{}, fmt.Errorf("opening link state cookie: %w", err)
	}

	var c linkStateCookie
	if err := json.Unmarshal(payload, &c); err != nil {
		return linkStateCookie{}, fmt.Errorf("decoding link state cookie: %w", err)
	}

	if time.Now().After(c.Expiry) {
		return linkStateCookie{}, linkExpiredf("state cookie")
	}

	return c, nil
}

// writeLinkStateCookie sets or clears the state cookie. A negative maxAge expires it.
func (s *Server) writeLinkStateCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     linkStateCookieName,
		Value:    value,
		Path:     linkPath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func newNonce() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// writeLinkCookie sets or clears the handoff cookie. A negative maxAge expires it.
func (s *Server) writeLinkCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     linkCookieName,
		Value:    value,
		Path:     linkPath,
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
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	lt, err := s.openLinkToken(encoded, linkTokenAAD)
	if err != nil {
		s.logger.Warn("rejecting link token", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	nonce, err := newNonce()
	if err != nil {
		s.logger.Error("generating nonce", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	lt.Nonce = nonce

	state, err := s.sealLinkToken(lt, linkStateAAD)
	if err != nil {
		s.logger.Error("sealing link state", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	sealedState, err := s.sealLinkStateCookie(nonce)
	if err != nil {
		s.logger.Error("sealing link state cookie", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	s.writeLinkStateCookie(w, sealedState, int(s.cfg.LinkCookieLifetime.Seconds()))

	http.Redirect(w, r, s.githubClient.AuthorizeURL(state), http.StatusFound)
}

// --- GET /link/github/callback?code=...&state=... ---.
func (s *Server) handleLinkGitHubCallback(w http.ResponseWriter, r *http.Request) {
	// Single use: expire the cookie now; this request already carries it, and
	// every path below must consume it, including a missing or invalid state.
	s.writeLinkStateCookie(w, "", -1)

	ctx := r.Context()
	state := r.URL.Query().Get("state")
	if state == "" {
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	lt, err := s.openLinkToken(state, linkStateAAD)
	if err != nil {
		s.logger.Warn("rejecting github callback state", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	cookie, err := r.Cookie(linkStateCookieName)
	if err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	binding, err := s.openLinkStateCookie(cookie.Value)
	if err != nil {
		s.logger.Warn("rejecting link state cookie", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	if lt.Nonce == "" || binding.Nonce == "" ||
		subtle.ConstantTimeCompare([]byte(binding.Nonce), []byte(lt.Nonce)) != 1 {
		s.logger.Warn("link state nonce mismatch")
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	code := r.URL.Query().Get("code")

	accessToken, err := s.githubClient.Exchange(ctx, code)
	if err != nil {
		s.logger.Error("github oauth exchange failed", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailGitHubAuth)
		return
	}

	ghUser, err := s.githubClient.FetchUser(ctx, accessToken)
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

	lc := linkCookie{
		RepositoryID:      lt.RepositoryID,
		GitHubUserID:      lt.GitHubUserID,
		Repository:        lt.Repository,
		IssueNumber:       lt.IssueNumber,
		RunID:             lt.RunID,
		SkipLinkedComment: lt.SkipLinkedComment,
	}

	discordPath := url.URL{Path: "/link/discord"}
	discordStepURL := strings.TrimSuffix(s.cfg.PublicURL, "/") + discordPath.RequestURI()
	if err := s.syncSetupComment(
		ctx,
		lc.RepositoryID,
		lc.Repository,
		lc.IssueNumber,
		commentGitHubVerified,
		discordStepURL,
		!lc.SkipLinkedComment,
	); err != nil {
		s.logSetupCommentError(err)
	}

	s.redirectToDiscord(w, r, lc)
}

// --- GET /link/discord ---.
// Resumes a linking attempt at the Discord step, for a contributor who signed
// in with GitHub in this browser but left before authorizing Discord.
func (s *Server) handleLinkDiscord(w http.ResponseWriter, r *http.Request) {
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

	s.redirectToDiscord(w, r, lc)
}

// redirectToDiscord stores lc under a fresh state nonce in the handoff cookie
// and sends the browser to Discord's authorize page.
func (s *Server) redirectToDiscord(w http.ResponseWriter, r *http.Request, lc linkCookie) {
	nonce, err := newNonce()
	if err != nil {
		s.logger.Error("generating nonce", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	lc.Nonce = nonce

	sealedCookie, err := s.sealLinkCookie(lc)
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
	if state == "" || lc.Nonce == "" || subtle.ConstantTimeCompare([]byte(state), []byte(lc.Nonce)) != 1 {
		s.logger.Warn("link cookie nonce mismatch")
		s.writeProblem(w, r, http.StatusBadRequest, linkDetailRestart)
		return
	}

	code := r.URL.Query().Get("code")
	token, err := s.discordClient.Exchange(ctx, code)
	if err != nil {
		s.logger.Error("discord oauth exchange failed", "error", err)
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

	sealedAccess, err := s.storageSealer.Seal(
		[]byte(token.AccessToken),
		tokenAAD(discordUserID, aadFieldAccess),
	)
	if err != nil {
		s.logger.Error("sealing discord access token", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	sealedRefresh, err := s.storageSealer.Seal(
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
		if errors.Is(err, storage.ErrDiscordAlreadyLinked) {
			s.logger.Warn("discord account already linked to another github user",
				"repository_id", lc.RepositoryID,
				"discord_user_id", discordUserID,
			)
			s.writeProblem(w, r, http.StatusConflict, linkDetailDiscordTaken)
			return
		}

		s.logger.Error("persisting link", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, linkDetailInternal)
		return
	}

	s.writeLinkCookie(w, "", -1)

	if err := s.syncSetupComment(
		ctx,
		lc.RepositoryID,
		lc.Repository,
		lc.IssueNumber,
		commentLinked,
		"",
		!lc.SkipLinkedComment,
	); err != nil {
		s.logSetupCommentError(err)
	}

	s.rerunAfterLink(ctx, lc)

	prURL := fmt.Sprintf("https://github.com/%s/pull/%d", lc.Repository, lc.IssueNumber)
	http.Redirect(w, r, prURL, http.StatusSeeOther)
}

// rerunAfterLink re-runs the workflow run that posted the linking comment.
func (s *Server) rerunAfterLink(ctx context.Context, lc linkCookie) {
	if lc.RunID == 0 || lc.Repository == "" {
		return
	}

	token, err := s.githubClient.CreateInstallationToken(ctx, lc.RepositoryID, lc.Repository)
	if err != nil {
		s.logger.Warn(
			"skipping workflow rerun: cannot create installation token",
			"error", err,
			"repository", lc.Repository,
			"run_id", lc.RunID,
		)

		return
	}

	if err := s.githubClient.RerunWorkflow(ctx, token, lc.Repository, lc.RunID); err != nil {
		s.logger.Warn(
			"skipping workflow rerun: GitHub rejected the request",
			"error", err,
			"repository", lc.Repository,
			"run_id", lc.RunID,
		)

		return
	}

	s.logger.Info(
		"rerunning workflow after account link",
		"repository", lc.Repository,
		"run_id", lc.RunID,
	)
}
