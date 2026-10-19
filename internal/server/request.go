/*
Copyright 2026-present SonoLink.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

// readBody reads a request body no larger than limit.
func (s *Server) readBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "reading request body")

		return nil, false
	}

	if int64(len(body)) > limit {
		s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "request body too large")

		return nil, false
	}

	return body, true
}

// decodeBody reads a size-limited JSON request body into dst.
func (s *Server) decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, ok := s.readBody(w, r, maxRequestBytes)
	if !ok {
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
