package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sonolink/arbiterer/internal/github"
	"github.com/sonolink/arbiterer/internal/storage"
)

type closeRequest struct {
	IssueNumber int64 `json:"issue_number"` // GitHub's API addresses PRs as issues.
	RulesFailed bool  `json:"rules_failed,omitempty"`
}

type closeResponse struct {
	Closed   bool `json:"closed"`
	Recorded bool `json:"recorded"`
}

// handlePullRequestClose answers POST /v1/github/pulls/close.
func (s *Server) handlePullRequestClose(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := s.verifyBearer(w, r)
	if !ok {
		return
	}

	var req closeRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	if req.IssueNumber == 0 {
		s.writeProblem(w, r, http.StatusBadRequest, "issue_number is required")
		return
	}

	token, err := s.githubClient.CreateInstallationToken(ctx, claims.RepositoryID, claims.Repository)
	if err != nil {
		s.logger.Error("creating installation token", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	pr, err := s.githubClient.FetchPullRequest(ctx, token, claims.Repository, req.IssueNumber)
	if err != nil {
		s.logger.Error("fetching pull request", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	if req.RulesFailed && !pr.Merged {
		s.notifyRulesFailed(ctx, claims, req.IssueNumber)
	}

	if pr.State != "open" {
		// Already closed or merged: not the app's close to record or undo.
		s.writeJSON(w, http.StatusOK, closeResponse{})

		return
	}

	pr, err = s.githubClient.ClosePullRequest(ctx, token, claims.Repository, req.IssueNumber)
	if err != nil {
		s.logger.Error("closing pull request", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	recorded := true
	if err := s.store.UpsertAutoClosedPullRequest(ctx, claims.RepositoryID, req.IssueNumber, pr.ClosedAt); err != nil {
		recorded = false
		s.logger.Error(
			"recording auto-closed pull request",
			"error", err,
			"issue_number", req.IssueNumber,
		)
	}

	s.writeJSON(w, http.StatusOK, closeResponse{Closed: true, Recorded: recorded})
}

type rulesFailedRequest struct {
	IssueNumber int64 `json:"issue_number"` // GitHub's API addresses PRs as issues.
}

// handlePullRequestRulesFailed answers POST /v1/github/pulls/rules-failed.
func (s *Server) handlePullRequestRulesFailed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := s.verifyBearer(w, r)
	if !ok {
		return
	}

	var req rulesFailedRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	if req.IssueNumber == 0 {
		s.writeProblem(w, r, http.StatusBadRequest, "issue_number is required")
		return
	}

	token, err := s.githubClient.CreateInstallationToken(ctx, claims.RepositoryID, claims.Repository)
	if err != nil {
		s.logger.Error("creating installation token", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	pr, err := s.githubClient.FetchPullRequest(ctx, token, claims.Repository, req.IssueNumber)
	if err != nil {
		s.logger.Error("fetching pull request", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	if !pr.Merged {
		s.notifyRulesFailed(ctx, claims, req.IssueNumber)
	}

	s.writeJSON(w, http.StatusOK, struct{}{})
}

// notifyRulesFailed reconciles the setup comment to explain that the author did
// not satisfy the rules.
func (s *Server) notifyRulesFailed(ctx context.Context, claims *github.Claims, issueNumber int64) {
	if err := s.syncSetupComment(
		ctx,
		claims.RepositoryID,
		claims.Repository,
		issueNumber,
		commentRulesFailed,
		workflowRunURL(claims),
		false,
	); err != nil {
		s.logSetupCommentError(err)
	}
}

func workflowRunURL(claims *github.Claims) string {
	if claims.RunID == 0 {
		return ""
	}

	if claims.CheckRunID != 0 {
		return fmt.Sprintf(
			"https://github.com/%s/actions/runs/%d/job/%d",
			claims.Repository,
			claims.RunID,
			claims.CheckRunID,
		)
	}

	return fmt.Sprintf("https://github.com/%s/actions/runs/%d", claims.Repository, claims.RunID)
}

type openRequest struct {
	IssueNumber int64 `json:"issue_number"` // GitHub's API addresses PRs as issues.
}

type openResponse struct {
	Opened bool   `json:"opened"`
	Reason string `json:"reason,omitempty"`
}

// handlePullRequestOpen answers POST /v1/github/pulls/open: opens a pull request the app auto-closed.
func (s *Server) handlePullRequestOpen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := s.verifyBearer(w, r)
	if !ok {
		return
	}

	var req openRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	if req.IssueNumber == 0 {
		s.writeProblem(w, r, http.StatusBadRequest, "issue_number is required")
		return
	}

	resp, err := s.openIfAutoClosed(ctx, claims.RepositoryID, claims.Repository, req.IssueNumber)
	if err != nil {
		s.logger.Error(
			"opening pull request",
			"error", err,
			"issue_number", req.IssueNumber,
		)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")

		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

const reasonNotClosedByApp = "not closed by the app"

// openIfAutoClosed opens a pull request the app auto-closed and forgets the
// record. It is shared by the open endpoint and the after-link flow.
func (s *Server) openIfAutoClosed(ctx context.Context, repositoryID int64, repo string, issueNumber int64) (openResponse, error) {
	record, err := s.store.AutoClosedPullRequest(ctx, repositoryID, issueNumber)
	if errors.Is(err, storage.ErrNotFound) {
		// The server did not auto-close this pull request, so it never opens
		// it, no matter who closed it last.
		return openResponse{Reason: reasonNotClosedByApp}, nil
	}
	if err != nil {
		return openResponse{}, err
	}

	token, err := s.githubClient.CreateInstallationToken(ctx, repositoryID, repo)
	if err != nil {
		return openResponse{}, err
	}

	pr, err := s.githubClient.FetchPullRequest(ctx, token, repo, issueNumber)
	if err != nil {
		return openResponse{}, err
	}

	if pr.Merged {
		if err := s.store.DeleteAutoClosedPullRequest(ctx, repositoryID, issueNumber); err != nil {
			return openResponse{}, err
		}

		return openResponse{Reason: "merged"}, nil
	}

	if pr.State == "open" {
		if err := s.store.DeleteAutoClosedPullRequest(ctx, repositoryID, issueNumber); err != nil {
			return openResponse{}, err
		}

		return openResponse{Reason: "already open"}, nil
	}

	if !pr.ClosedAt.Truncate(time.Second).Equal(record.ClosedAt.Truncate(time.Second)) {
		if err := s.store.DeleteAutoClosedPullRequest(ctx, repositoryID, issueNumber); err != nil {
			return openResponse{}, err
		}

		return openResponse{Reason: "closed by someone else"}, nil
	}

	if err := s.githubClient.OpenPullRequest(ctx, token, repo, issueNumber); err != nil {
		return openResponse{}, err
	}

	if err := s.store.DeleteAutoClosedPullRequest(ctx, repositoryID, issueNumber); err != nil {
		return openResponse{}, err
	}

	return openResponse{Opened: true}, nil
}
