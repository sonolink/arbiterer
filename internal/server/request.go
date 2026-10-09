package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/sonolink/arbiterer/internal/github"
)

// verifyBearer authenticates a request by the repository's GitHub OIDC token.
func (s *Server) verifyBearer(w http.ResponseWriter, r *http.Request) (*github.Claims, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		s.writeProblem(w, r, http.StatusUnauthorized, "missing bearer token")

		return nil, false
	}

	claims, err := s.verifier.Verify(r.Context(), token)
	if err != nil {
		s.logger.Warn("rejecting oidc token", "error", err)
		s.writeProblem(w, r, http.StatusUnauthorized, "invalid token")

		return nil, false
	}

	return claims, true
}

// decodeBody reads a size-limited JSON request body into dst.
func (s *Server) decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "reading request body")

		return false
	}

	if len(body) > maxRequestBytes {
		s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "request body too large")

		return false
	}

	if err := json.Unmarshal(body, dst); err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid request body")

		return false
	}

	return true
}

// validGuildID reports whether id looks like a Discord snowflake.
func validGuildID(id string) bool {
	_, err := strconv.ParseUint(id, 10, 64)
	return err == nil
}
