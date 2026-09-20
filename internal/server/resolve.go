package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sonolink/arbiterer/internal/discord"
	"github.com/sonolink/arbiterer/internal/storage"
)

type resolveRequest struct {
	GitHubUserID string `json:"github_user_id"`
	GuildID      string `json:"guild_id"`
}

type resolveStatus string

const (
	statusLinked     resolveStatus = "linked"
	statusUnlinked   resolveStatus = "unlinked"
	statusRevoked    resolveStatus = "revoked"
	statusNotAMember resolveStatus = "not_a_member"
)

type resolveResponse struct {
	Status   resolveStatus   `json:"status"`
	Member   json.RawMessage `json:"member,omitempty"`
	SetupURL string          `json:"setup_url,omitempty"`
}

// setupURL builds the link a contributor follows to connect their accounts,
// carrying a sealed, short-lived bearer token.
func (s *Server) setupURL(githubUserID string, repositoryID int64) (string, error) {
	token, err := s.sealLinkToken(githubUserID, repositoryID)
	if err != nil {
		return "", fmt.Errorf("building setup url: %w", err)
	}

	u := url.URL{
		Path: "/link",
		RawQuery: url.Values{
			"token": {token},
		}.Encode(),
	}

	return strings.TrimSuffix(s.cfg.PublicURL, "/") + u.RequestURI(), nil
}

const (
	aadFieldAccess  = "access"
	aadFieldRefresh = "refresh"
)

// tokenAAD builds the additional data that binds a sealed token to the row and
// column holding it. Sealing and opening a token must use the same value.
func tokenAAD(discordUserID int64, field string) []byte {
	return []byte(strconv.FormatInt(discordUserID, 10) + ":" + field)
}

// errReauthRequired reports that the stored Discord grant can no longer be
// used, so the user has to authorize again.
var errReauthRequired = errors.New("server: discord grant is no longer usable")

const (
	// refreshSkew is how early a token is treated as expired, absorbing clock differences and
	// the round trip to Discord.
	refreshSkew = time.Minute

	// tokenStoreTimeout bounds the write that persists refreshed tokens, which
	// must outlive the request that triggered it.
	tokenStoreTimeout = 5 * time.Second

	// tokenStoreAttempts is how many times a refreshed token is stored before the
	// grant is treated as lost.
	tokenStoreAttempts = 3

	// tokenStoreBackoff is the wait after the first failed store, doubling with
	// each further attempt.
	tokenStoreBackoff = 50 * time.Millisecond
)

func (s *Server) accessToken(ctx context.Context, user *storage.DiscordUser) (string, error) {
	if time.Until(user.TokenExpiresAt) > refreshSkew {
		accessToken, err := s.sealer.Open(
			user.EncryptedAccessToken,
			tokenAAD(user.ID, aadFieldAccess),
		)
		if err != nil {
			return "", fmt.Errorf("opening access token: %w", err)
		}

		return string(accessToken), nil
	}

	refreshToken, err := s.sealer.Open(
		user.EncryptedRefreshToken,
		tokenAAD(user.ID, aadFieldRefresh),
	)
	if err != nil {
		return "", fmt.Errorf("opening refresh token: %w", err)
	}

	token, err := s.discordClient.Refresh(ctx, string(refreshToken))
	if err != nil {
		// Only invalid_grant means the stored grant is no longer usable. The
		// other OAuth error codes report a misconfigured client, which is our
		// problem, not the user's.
		var oauthErr *discord.OAuthError
		if errors.As(err, &oauthErr) && oauthErr.Code == "invalid_grant" {
			return "", errReauthRequired
		}

		return "", fmt.Errorf("refreshing token: %w", err)
	}

	sealedAccessToken, err := s.sealer.Seal(
		[]byte(token.AccessToken),
		tokenAAD(user.ID, aadFieldAccess),
	)
	if err != nil {
		return "", fmt.Errorf("sealing access token: %w", err)
	}

	sealedRefreshToken, err := s.sealer.Seal(
		[]byte(token.RefreshToken),
		tokenAAD(user.ID, aadFieldRefresh),
	)
	if err != nil {
		return "", fmt.Errorf("sealing refresh token: %w", err)
	}

	user.EncryptedAccessToken = sealedAccessToken
	user.EncryptedRefreshToken = sealedRefreshToken
	user.TokenExpiresAt = token.ExpiresAt

	// Discord has already invalidated the old refresh token, so this write has to outlive
	// the request that triggered it.
	storeCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		tokenStoreTimeout,
	)
	defer cancel()

	if err := s.storeRefreshedTokens(storeCtx, user); err != nil {
		return "", err
	}

	return token.AccessToken, nil
}

// storeRefreshedTokens persists refreshed credentials, retrying transient
// failures. Losing this write costs the user their grant, since Discord has
// already invalidated the token it replaced.
func (s *Server) storeRefreshedTokens(ctx context.Context, user *storage.DiscordUser) error {
	var err error

	backoff := tokenStoreBackoff

loop:
	for attempt := 1; attempt <= tokenStoreAttempts; attempt++ {
		err = s.store.UpdateDiscordUserTokens(ctx, user)
		if err == nil {
			return nil
		}

		if errors.Is(err, storage.ErrNotFound) {
			break
		}

		if attempt < tokenStoreAttempts {
			select {
			case <-ctx.Done():
				err = ctx.Err()
				break loop
			case <-time.After(backoff):
				backoff *= 2
			}
		}
	}

	s.logger.Error(
		"lost refreshed discord tokens",
		"discord_user_id", user.ID,
		"error", err,
	)

	return fmt.Errorf("storing refreshed tokens: %w", err)
}

func (s *Server) resolveMember(
	ctx context.Context,
	user *storage.DiscordUser,
	guildID string,
	githubUserID string,
	repositoryID int64,
) (resolveResponse, error) {
	accessToken, err := s.accessToken(ctx, user)
	if err != nil {
		if errors.Is(err, errReauthRequired) {
			return s.revokedResponse(githubUserID, repositoryID)
		}

		return resolveResponse{}, err
	}

	member, err := s.discordClient.GuildMember(ctx, accessToken, guildID)
	if err == nil {
		return resolveResponse{Status: statusLinked, Member: member}, nil
	}

	var apiErr *discord.APIError
	if !errors.As(err, &apiErr) {
		return resolveResponse{}, fmt.Errorf("fetching guild member: %w", err)
	}

	switch apiErr.Status {
	case http.StatusUnauthorized:
		return s.revokedResponse(githubUserID, repositoryID)
	case http.StatusNotFound:
		return resolveResponse{Status: statusNotAMember}, nil
	default:
		return resolveResponse{}, fmt.Errorf("fetching guild member: %w", err)
	}
}

// revokedResponse builds the response served when a Discord grant is unusable,
// pointing the user at the linking flow.
func (s *Server) revokedResponse(githubUserID string, repositoryID int64) (resolveResponse, error) {
	setupURL, err := s.setupURL(githubUserID, repositoryID)
	if err != nil {
		return resolveResponse{}, err
	}

	return resolveResponse{
		Status:   statusRevoked,
		SetupURL: setupURL,
	}, nil
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		s.writeProblem(w, r, http.StatusUnauthorized, "missing bearer token")
		return
	}

	claims, err := s.verifier.Verify(ctx, token)
	if err != nil {
		s.logger.Warn("rejecting oidc token", "error", err)
		s.writeProblem(w, r, http.StatusUnauthorized, "invalid token")
		return
	}

	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.GitHubUserID == "" {
		s.writeProblem(w, r, http.StatusBadRequest, "github_user_id is required")
		return
	}

	user, err := s.store.DiscordUserByConnection(
		ctx,
		req.GitHubUserID,
		claims.RepositoryID,
	)
	if errors.Is(err, storage.ErrNotFound) {
		setupURL, err := s.setupURL(req.GitHubUserID, claims.RepositoryID)
		if err != nil {
			s.logger.Error("building setup url", "error", err)
			s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
			return
		}

		s.writeJSON(w, http.StatusOK, resolveResponse{
			Status:   statusUnlinked,
			SetupURL: setupURL,
		})
		return
	}

	if err != nil {
		s.logger.Error("looking up connection", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	if req.GuildID == "" {
		s.writeJSON(w, http.StatusOK, resolveResponse{Status: statusLinked})
		return
	}

	resp, err := s.resolveMember(ctx, user, req.GuildID, req.GitHubUserID, claims.RepositoryID)
	if err != nil {
		s.logger.Error("resolving member", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}
