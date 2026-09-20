package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	// linkTokenAAD is the fixed context that seals link tokens. Tokens are
	// bearer-style and not bound to a stored row, so unlike tokenAAD there is
	// no per-user component.
	linkTokenAAD = "link-token"

	// linkCookieAAD seals the intermediate linking cookie the same way.
	linkCookieAAD = "link-cookie"

	// linkCookieName is the intermediate cookie the GitHub leg sets.
	linkCookieName = "arbiterer_link"

	// linkDiscordScopes must cover what resolveMember needs later:
	// identify for the user id, guilds.members.read for /users/@me/guilds/{id}/member.
	linkDiscordScopes = "identify guilds.members.read"
)

var errLinkExpired = errors.New("link token expired")

// linkToken is the sealed payload carried through the URL from /v1/resolve
type linkToken struct {
	GitHubUserID string    `json:"github_user_id"`
	RepositoryID int64     `json:"repository_id"`
	Expiry       time.Time `json:"expiry"`
}

// linkCookie is the sealed payload carried between the GitHub and Discord.
type linkCookie struct {
	Nonce        string `json:"nonce"`
	RepositoryID int64  `json:"repository_id"`
	GitHubUserID string `json:"github_user_id"`
}

// sealLinkToken produces a URL-safe bearer token for a resolving link.
func (s *Server) sealLinkToken(githubUserID string, repositoryID int64) (string, error) {
	payload, err := json.Marshal(linkToken{
		GitHubUserID: githubUserID,
		RepositoryID: repositoryID,
		Expiry:       time.Now().Add(s.cfg.LinkTokenLifetime),
	})

	if err != nil {
		return "", fmt.Errorf("sealing link token: %w", err)
	}

	sealed, err := s.sealer.Seal(payload, []byte(linkTokenAAD))

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

	payload, err := s.sealer.Open(sealed, []byte(linkTokenAAD))
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

	sealed, err := s.sealer.Seal(payload, []byte(linkCookieAAD))
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

	payload, err := s.sealer.Open(sealed, []byte(linkCookieAAD))
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

func (s *Server) setLinkCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     linkCookieName,
		Value:    value,
		Path:     "/link",
		MaxAge:   int(s.cfg.LinkCookieLifetime.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearLinkCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     linkCookieName,
		Value:    "",
		Path:     "/link",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true, // should be false for localhost in development
		SameSite: http.SameSiteLaxMode,
	})
}

// --- GET /link?token=... ---
func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	encoded := r.URL.Query().Get("token")
	if encoded == "" {
		s.writeProblem(w, r, http.StatusBadRequest, "This link is invalid or has expired. Please re-run the check to generate a fresh one.")
		return
	}

	_, err := s.openLinkToken(encoded)
	if errors.Is(err, errLinkExpired) {
		s.writeProblem(w, r, http.StatusBadRequest, "This link has expired. Please re-run the check to get a fresh one.")
		return
	}

	if err != nil {
		s.logger.Warn("rejecting link token", "error", err)
		s.writeProblem(w, r, http.StatusBadRequest, "This link is invalid or has expired. Please re-run the check to generate a fresh one.")
		return
	}

	http.Redirect(w, r, s.githubClient.AuthorizeURL(encoded), http.StatusFound)
}

