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
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sonolink/arbiterer/internal/github"
	"github.com/sonolink/arbiterer/internal/storage"
)

type pullRequestRequest struct {
	IssueNumber int64 `json:"issue_number"` // GitHub's API addresses pull requests as issues.
}

func (pr pullRequestRequest) issueNumber() int64 {
	return pr.IssueNumber
}

type closeRequest struct {
	pullRequestRequest
	RulesFailed bool `json:"rules_failed,omitempty"`
}

type closeResponse struct {
	Closed   bool `json:"closed"`
	Recorded bool `json:"recorded"`
}

// issueRequest is implemented by every pull request action body.
type issueRequest interface {
	issueNumber() int64
}

// decodePullRequestRequest authenticates the caller and decodes a pull request
// action body, rejecting a missing issue number.
func (s *Server) decodePullRequestRequest(
	w http.ResponseWriter,
	r *http.Request,
	req issueRequest,
) (*github.Claims, bool) {
	claims, ok := s.verifyBearer(w, r)
	if !ok {
		return nil, false
	}

	if !s.decodeBody(w, r, req) {
		return nil, false
	}

	if req.issueNumber() == 0 {
		s.writeProblem(w, r, http.StatusBadRequest, "issue_number is required")

		return nil, false
	}

	return claims, true
}

// loadPullRequest creates an installation token and fetches the pull request.
func (s *Server) loadPullRequest(
	ctx context.Context,
	repositoryID int64,
	repo string,
	issueNumber int64,
) (string, *github.PullRequest, error) {
	token, err := s.githubClient.CreateInstallationToken(ctx, repositoryID, repo)
	if err != nil {
		return "", nil, fmt.Errorf("creating installation token: %w", err)
	}

	pr, err := s.githubClient.FetchPullRequest(ctx, token, repo, issueNumber)
	if err != nil {
		return "", nil, fmt.Errorf("fetching pull request: %w", err)
	}

	return token, pr, nil
}

// workflowRunURL returns the Actions run or job URL for the workflow run.
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

// sameClosedAt reports whether two close times name the same second, which is
// the precision GitHub keeps across payloads.
func sameClosedAt(a, b time.Time) bool {
	return a.Truncate(time.Second).Equal(b.Truncate(time.Second))
}

// handleGitHubPullRequestClose answers POST /v1/github/pulls/close.
func (s *Server) handleGitHubPullRequestClose(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req closeRequest

	claims, ok := s.decodePullRequestRequest(w, r, &req)
	if !ok {
		return
	}

	token, pr, err := s.loadPullRequest(ctx, claims.RepositoryID, claims.Repository, req.IssueNumber)
	if err != nil {
		s.logger.Error(
			"loading pull request",
			"error", err,
			"repository", claims.Repository,
			"issue_number", req.IssueNumber,
		)
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

// handleGitHubPullRequestRulesFailed answers POST /v1/github/pulls/rules-failed.
func (s *Server) handleGitHubPullRequestRulesFailed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req pullRequestRequest

	claims, ok := s.decodePullRequestRequest(w, r, &req)
	if !ok {
		return
	}

	_, pr, err := s.loadPullRequest(ctx, claims.RepositoryID, claims.Repository, req.IssueNumber)
	if err != nil {
		s.logger.Error(
			"loading pull request",
			"error", err,
			"repository", claims.Repository,
			"issue_number", req.IssueNumber,
		)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")

		return
	}

	if !pr.Merged {
		s.notifyRulesFailed(ctx, claims, req.IssueNumber)
	}

	s.writeJSON(w, http.StatusOK, struct{}{})
}

// handleGitHubPullRequestPassed answers POST /v1/github/pulls/passed.
func (s *Server) handleGitHubPullRequestPassed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req pullRequestRequest

	claims, ok := s.decodePullRequestRequest(w, r, &req)
	if !ok {
		return
	}

	s.notifyPassed(ctx, claims, req.IssueNumber)

	s.writeJSON(w, http.StatusOK, struct{}{})
}

type openResponse struct {
	Opened bool   `json:"opened"`
	Reason string `json:"reason,omitempty"`
}

// handleGitHubPullRequestOpen answers POST /v1/github/pulls/open: opens a pull request the app auto-closed.
func (s *Server) handleGitHubPullRequestOpen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req pullRequestRequest

	claims, ok := s.decodePullRequestRequest(w, r, &req)
	if !ok {
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

	token, pr, err := s.loadPullRequest(ctx, repositoryID, repo, issueNumber)
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

	if !sameClosedAt(pr.ClosedAt, record.ClosedAt) {
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
