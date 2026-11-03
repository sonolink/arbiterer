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
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

// workflowFile returns the workflow file that ran the check and the ref to link
// it at. Only the pull request's own repository and a non-pull ref are accepted.
func workflowFile(claims *github.Claims) (path, ref string, ok bool) {
	at := strings.LastIndex(claims.WorkflowRef, "@refs/")
	if at < 0 {
		return "", "", false
	}

	parts := strings.SplitN(claims.WorkflowRef[:at], "/", 3)
	if len(parts) != 3 || !strings.EqualFold(parts[0]+"/"+parts[1], claims.Repository) {
		return "", "", false
	}

	path = parts[2]
	if !strings.HasPrefix(path, ".github/workflows/") ||
		(!strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml")) ||
		strings.Contains(path, "..") {
		return "", "", false
	}

	if sha := claims.WorkflowSHA; len(sha) == 40 {
		if _, err := hex.DecodeString(sha); err == nil {
			return path, sha, true
		}
	}

	ref = strings.TrimPrefix(claims.WorkflowRef[at+1:], "refs/heads/")
	if ref == "" || strings.HasPrefix(ref, "refs/") {
		return "", "", false
	}

	return path, ref, true
}

// isRulesBlock reports whether line opens an inline rules block.
func isRulesBlock(line string) bool {
	key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
	if !ok || strings.TrimSpace(key) != "rules" {
		return false
	}

	value = strings.TrimSpace(value)

	return strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">")
}

// rulesLines returns the 1-based range of the first inline rules block.
func rulesLines(source string) (start, end int, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")

	for i, line := range lines {
		if !isRulesBlock(line) {
			continue
		}

		keyIndent := len(line) - len(strings.TrimLeft(line, " \t"))
		start, end = i+2, i+1

		for j := i + 1; j < len(lines); j++ {
			body := lines[j]
			if strings.TrimSpace(body) == "" {
				continue
			}

			if len(body)-len(strings.TrimLeft(body, " \t")) <= keyIndent {
				break
			}

			end = j + 1
		}

		if end >= start {
			return start, end, true
		}

		return 0, 0, false
	}

	return 0, 0, false
}

// rulesFileURL links the rules a failed pull request was judged by, falling back
// to the workflow file, or to no link, when the block cannot be resolved.
func (s *Server) rulesFileURL(ctx context.Context, token string, claims *github.Claims) string {
	path, ref, ok := workflowFile(claims)
	if !ok {
		return ""
	}

	base := fmt.Sprintf(
		"https://github.com/%s/blob/%s/%s",
		claims.Repository,
		escapeURLPath(ref),
		escapeURLPath(path),
	)
	if token == "" {
		return base
	}

	source, err := s.githubClient.FetchFileContents(ctx, token, claims.Repository, path, ref)
	if err != nil {
		s.logger.Debug(
			"fetching the workflow file for the rules link",
			"error", err,
			"repository", claims.Repository,
			"path", path,
		)

		return base
	}

	start, end, ok := rulesLines(source)
	if !ok {
		return base
	}

	if start == end {
		return fmt.Sprintf("%s#L%d", base, start)
	}

	return fmt.Sprintf("%s#L%d-L%d", base, start, end)
}

// escapeURLPath escapes each segment of a repository path or ref, keeping the
// slashes that separate them.
func escapeURLPath(s string) string {
	segments := strings.Split(s, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}

	return strings.Join(segments, "/")
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
		s.notifyRulesFailed(ctx, claims, req.IssueNumber, token)
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

	if !pr.Merged {
		s.notifyRulesFailed(ctx, claims, req.IssueNumber, token)
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
