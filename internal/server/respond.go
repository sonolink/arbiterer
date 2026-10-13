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
	"net/http"
)

const (
	contentTypeJSON    = "application/json"
	contentTypeProblem = "application/problem+json"
)

// writeJSON sends v as a JSON response body.
//
//nolint:unparam // other codes are expected.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	s.write(w, status, contentTypeJSON, v)
}

// write sends v as a response body with the given status and content type.
func (s *Server) write(w http.ResponseWriter, status int, contentType string, v any) {
	// Marshal before sending any headers, so an encoding failure can still be
	// reported rather than truncating a response that has already begun.
	body, err := json.Marshal(v)
	if err != nil {
		s.logger.Error("encoding response body", "error", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		s.logger.Error("writing response body", "error", err)
	}
}
