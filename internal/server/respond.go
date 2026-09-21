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
//nolint:unparam // status is temporarily currently always 200
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
