package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sonolink/arbiterer/internal/discord"
	"github.com/sonolink/arbiterer/internal/storage"
)

type memberRequest struct {
	GitHubUserID string `json:"github_user_id"`
	GuildID      string `json:"guild_id"`
}

type memberResponse struct {
	// Member is the raw member record, or nil when the account is not in the
	// requested guild. A non-member is an answer, not an error.
	Member  json.RawMessage `json:"member"`
	Status  resolveStatus   `json:"status"`
	LinkURL string          `json:"link_url,omitempty"`
}

// handleMember answers POST /v1/member, reading the guild member record of the
// Discord account linked to a GitHub user.
func (s *Server) handleMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := s.verifyBearer(w, r)
	if !ok {
		return
	}

	var req memberRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	switch {
	case req.GitHubUserID == "":
		s.writeProblem(w, r, http.StatusBadRequest, "github_user_id is required")

		return
	case !validGuildID(req.GuildID):
		s.writeProblem(w, r, http.StatusBadRequest, "guild_id must be a Discord server id")

		return
	}

	user, err := s.store.DiscordUserByConnection(ctx, req.GitHubUserID, claims.RepositoryID)

	switch {
	case errors.Is(err, storage.ErrNotFound):
		s.writeProblem(w, r, http.StatusNotFound, "no linked Discord account for this GitHub user")

		return
	case err != nil:
		s.logger.Error("looking up connection", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")

		return
	}

	s.writeMember(w, r, user, req.GuildID, linkToken{
		GitHubUserID: req.GitHubUserID,
		RepositoryID: claims.RepositoryID,
		Repository:   claims.Repository,
	})
}

// writeMember reads the member record for one guild and writes the response.
func (s *Server) writeMember(
	w http.ResponseWriter,
	r *http.Request,
	user *storage.DiscordUser,
	guildID string,
	lt linkToken,
) {
	ctx := r.Context()

	accessToken, err := s.accessToken(ctx, user)
	if err != nil {
		if !errors.Is(err, errReauthRequired) {
			s.logger.Error("reading access token", "error", err)
			s.writeProblem(w, r, http.StatusInternalServerError, "internal error")

			return
		}

		s.writeRevoked(w, r, lt)

		return
	}

	member, err := s.discordClient.GuildMember(ctx, accessToken, guildID)
	if err == nil {
		s.writeJSON(w, http.StatusOK, memberResponse{Status: statusLinked, Member: member})

		return
	}

	var apiErr *discord.APIError
	if !errors.As(err, &apiErr) {
		s.logger.Error("fetching guild member", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")

		return
	}

	switch apiErr.Status {
	case http.StatusUnauthorized:
		s.writeRevoked(w, r, lt)
	case http.StatusNotFound:
		s.writeJSON(w, http.StatusOK, memberResponse{Status: statusNotAMember})
	default:
		s.logger.Error("fetching guild member", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
	}
}

// writeRevoked reports an unusable grant, pointing the caller at the linking flow.
func (s *Server) writeRevoked(w http.ResponseWriter, r *http.Request, lt linkToken) {
	linkURL, err := s.linkURL(lt)
	if err != nil {
		s.logger.Error("building link url", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")

		return
	}

	s.writeJSON(w, http.StatusOK, memberResponse{Status: statusRevoked, LinkURL: linkURL})
}
