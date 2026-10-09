package server

import (
	"net/http"
)

type closedRequest struct {
	IssueNumber int64 `json:"issue_number"` // GitHub's API addresses PRs as issues.
	Closed      bool  `json:"closed"`
}

// handleClosed answers POST /v1/pulls/closed: records that the action closed
// the pull request, or forgets it when the pull request is open again.
func (s *Server) handleClosed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := s.verifyBearer(w, r)
	if !ok {
		return
	}

	var req closedRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	if req.IssueNumber == 0 {
		s.writeProblem(w, r, http.StatusBadRequest, "issue_number is required")
		return
	}

	var err error
	if req.Closed {
		err = s.store.UpsertAutoClosedPullRequest(ctx, claims.RepositoryID, req.IssueNumber, claims.RunID)
	} else {
		err = s.store.DeleteAutoClosedPullRequest(ctx, claims.RepositoryID, req.IssueNumber)
	}

	if err != nil {
		s.logger.Error(
			"recording close by arbiterer",
			"error", err,
			"issue_number", req.IssueNumber,
		)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]bool{"closed_by_arbiterer": req.Closed})
}
