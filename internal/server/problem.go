package server

import (
	"net/http"
)

// problemDetails is an RFC 9457 error response. The type member is omitted,
// which the RFC defines as equivalent to "about:blank".
type problemDetails struct {
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// writeProblem sends an RFC 9457 problem response describing a failed request.
func (s *Server) writeProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	s.write(w, status, contentTypeProblem, problemDetails{
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
	})
}
