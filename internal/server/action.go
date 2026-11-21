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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sonolink/arbiterer/internal/storage"
)

// commentAction is an action the PR author can take from the setup comment.
type commentAction string

const (
	actionRecheck commentAction = "recheck"
	actionUnlink  commentAction = "unlink"
	actionDelete  commentAction = "delete"
)

// actionPath is under linkPath so the state cookie reaches the GitHub callback.
const actionPath = linkPath + "/action"

const (
	actionDetailExpired  = "This link has expired. Please re-run the check to get a new one."
	actionDetailClosed   = "This pull request can no longer be rechecked."
	actionDetailCooldown = "A recheck was started recently. Please try again in a few minutes."
	actionDetailNoRun    = "There is no check run to recheck yet."
	actionDetailRerun    = "Could not start the recheck. Please try again."
)

// commentActions holds the action links shown in the setup comment.
type commentActions struct {
	Recheck string
	Unlink  string
	Delete  string
}

// commentActionsFor builds the action links for a comment with the given
// status. Recheck is only offered when the rules failed.
func (s *Server) commentActionsFor(
	status commentStatus,
	repositoryID int64,
	repo string,
	issueNumber int64,
	authorID int64,
) (commentActions, error) {
	var actions commentActions

	switch status {
	case commentLinked, commentPassed, commentRulesFailed, commentRechecking:
	default:
		return actions, nil
	}

	lt := linkToken{
		GitHubUserID: strconv.FormatInt(authorID, 10),
		RepositoryID: repositoryID,
		Repository:   repo,
		IssueNumber:  issueNumber,
	}

	var err error

	if status == commentRulesFailed {
		actions.Recheck, err = s.actionURL(lt, actionRecheck)
		if err != nil {
			return commentActions{}, err
		}
	}

	actions.Unlink, err = s.actionURL(lt, actionUnlink)
	if err != nil {
		return commentActions{}, err
	}

	actions.Delete, err = s.actionURL(lt, actionDelete)
	if err != nil {
		return commentActions{}, err
	}

	return actions, nil
}

// actionURL builds the link that runs action for the author in lt.
func (s *Server) actionURL(lt linkToken, action commentAction) (string, error) {
	lt.Action = action

	token, err := s.sealLinkTokenFor(lt, actionTokenAAD, s.cfg.ActionTokenLifetime)
	if err != nil {
		return "", fmt.Errorf("building %s url: %w", action, err)
	}

	u := url.URL{
		Path: actionPath,
		RawQuery: url.Values{
			tokenParam: {token},
		}.Encode(),
	}

	return strings.TrimSuffix(s.cfg.PublicURL, "/") + u.RequestURI(), nil
}

// --- GET /v1/link/action?token=... ---.
func (s *Server) handleCommentAction(w http.ResponseWriter, r *http.Request) {
	encoded := r.URL.Query().Get(tokenParam)
	if encoded == "" {
		s.actionError(w, linkToken{}, actionDetailExpired)
		return
	}

	lt, err := s.openLinkToken(encoded, actionTokenAAD)
	if err != nil {
		s.logger.Warn("rejecting action token", "error", err)
		s.actionError(w, lt, actionDetailExpired)
		return
	}

	switch lt.Action {
	case actionRecheck, actionUnlink, actionDelete:
	default:
		s.logger.Warn("rejecting action token", "action", lt.Action)
		s.actionError(w, lt, actionDetailExpired)
		return
	}

	s.startGitHubAuth(w, r, lt)
}

// runCommentAction runs the action once the GitHub callback has verified the
// author. A recheck runs straight away; unlinking and deleting go to the
// confirmation page first.
func (s *Server) runCommentAction(w http.ResponseWriter, r *http.Request, lt linkToken) {
	ctx := r.Context()

	switch lt.Action {
	case actionRecheck:
		if detail := s.recheckFromComment(ctx, lt); detail != "" {
			s.actionError(w, lt, detail)
			return
		}

		if err := s.syncSetupComment(
			ctx,
			lt.RepositoryID,
			lt.Repository,
			lt.IssueNumber,
			commentRechecking,
			"",
			false,
		); err != nil {
			s.logSetupCommentError(err)
		}

		http.Redirect(w, r, pullRequestURL(lt), http.StatusSeeOther)
	case actionUnlink, actionDelete:
		token, err := s.sealLinkToken(lt, actionConfirmAAD)
		if err != nil {
			s.logger.Error("sealing action confirm token", "error", err)
			s.actionError(w, lt, linkDetailInternal)
			return
		}

		s.writeActionPage(w, http.StatusOK, actionPage{
			Action:  string(lt.Action),
			Repo:    lt.Repository,
			PRURL:   pullRequestURL(lt),
			PostURL: actionPath + "/confirm",
			Token:   token,
		})
	default:
		s.actionError(w, lt, actionDetailExpired)
	}
}

// --- POST /v1/link/action/confirm ---.
// The confirmation page posts here once the author confirms.
func (s *Server) handleCommentActionConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	lt, err := s.openLinkToken(r.PostFormValue(tokenParam), actionConfirmAAD)
	if err != nil {
		s.logger.Warn("rejecting action confirm token", "error", err)
		s.actionError(w, lt, actionDetailExpired)
		return
	}

	status := commentUnlinked

	var users []storage.DiscordUser

	switch lt.Action {
	case actionUnlink:
		users, err = s.store.UnlinkGitHubUser(ctx, lt.GitHubUserID, lt.RepositoryID)
		if err != nil {
			s.logger.Error(
				"unlinking github user",
				"error", err,
				"repository", lt.Repository,
			)
			s.actionError(w, lt, linkDetailInternal)
			return
		}
	case actionDelete:
		users, err = s.store.DeleteGitHubUser(ctx, lt.GitHubUserID)
		if err != nil {
			s.logger.Error("deleting github user", "error", err)
			s.actionError(w, lt, linkDetailInternal)
			return
		}

		status = commentDeleted
	default:
		s.actionError(w, lt, actionDetailExpired)
		return
	}

	s.revokeDiscordTokens(ctx, users)

	s.logger.Info(
		"ran comment action",
		"action", lt.Action,
		"repository", lt.Repository,
		"issue_number", lt.IssueNumber,
	)

	s.relinkComment(ctx, lt, status)
	s.rerunAfterAction(ctx, lt)

	http.Redirect(w, r, pullRequestURL(lt), http.StatusSeeOther)
}

// relinkComment rewrites the setup comment to ask the author to link again.
func (s *Server) relinkComment(ctx context.Context, lt linkToken, status commentStatus) {
	linkURL, err := s.linkURL(linkToken{
		GitHubUserID: lt.GitHubUserID,
		RepositoryID: lt.RepositoryID,
		Repository:   lt.Repository,
		IssueNumber:  lt.IssueNumber,
	})
	if err != nil {
		s.logger.Error("building link url", "error", err)
		return
	}

	if err := s.syncSetupComment(
		ctx,
		lt.RepositoryID,
		lt.Repository,
		lt.IssueNumber,
		status,
		linkURL,
		false,
	); err != nil {
		s.logSetupCommentError(err)
	}
}

// revokeDiscordTokens revokes the tokens of the removed Discord users.
func (s *Server) revokeDiscordTokens(ctx context.Context, users []storage.DiscordUser) {
	for _, user := range users {
		accessToken, err := s.openAccessToken(&user)
		if err != nil {
			s.logger.Error("opening access token", "error", err)
			continue
		}

		if err := s.discordClient.RevokeToken(ctx, accessToken); err != nil {
			s.logger.Warn("revoking discord token", "error", err)
		}
	}
}

// rerunAfterAction reruns the pull request's checks, ignoring the recheck
// cooldown, so they see the author is no longer linked.
func (s *Server) rerunAfterAction(ctx context.Context, lt linkToken) {
	token, err := s.githubClient.CreateInstallationToken(ctx, lt.RepositoryID, lt.Repository)
	if err != nil {
		s.logger.Warn(
			"skipping rerun: cannot create installation token",
			"error", err,
			"repository", lt.Repository,
		)

		return
	}

	_, _ = s.rerunPullRequest(ctx, token, lt.RepositoryID, lt.Repository, lt.IssueNumber, 0)
}

// pullRequestURL returns the GitHub page of the pull request in lt.
func pullRequestURL(lt linkToken) string {
	return fmt.Sprintf("https://github.com/%s/pull/%d", lt.Repository, lt.IssueNumber)
}

// recheckFromComment reruns the pull request's workflow run. It returns the
// reason the rerun did not start, or an empty string when it did.
func (s *Server) recheckFromComment(ctx context.Context, lt linkToken) string {
	token, err := s.githubClient.CreateInstallationToken(ctx, lt.RepositoryID, lt.Repository)
	if err != nil {
		s.logger.Warn(
			"skipping recheck: cannot create installation token",
			"error", err,
			"repository", lt.Repository,
		)

		return actionDetailRerun
	}

	pr, err := s.githubClient.FetchPullRequest(ctx, token, lt.Repository, lt.IssueNumber)
	if err != nil {
		s.logger.Warn(
			"skipping recheck: cannot fetch pull request",
			"error", err,
			"repository", lt.Repository,
			"issue_number", lt.IssueNumber,
		)

		return actionDetailRerun
	}

	if pr.Merged {
		return actionDetailClosed
	}

	if pr.State == "closed" {
		closedByApp, err := s.closedByApp(ctx, lt.RepositoryID, lt.IssueNumber, &pr.ClosedAt)
		if err != nil {
			s.logger.Error(
				"checking whether the app closed the pull request",
				"error", err,
				"repository", lt.Repository,
				"issue_number", lt.IssueNumber,
			)

			return linkDetailInternal
		}

		if !closedByApp {
			return actionDetailClosed
		}
	}

	runID, err := s.rerunPullRequest(
		ctx,
		token,
		lt.RepositoryID,
		lt.Repository,
		lt.IssueNumber,
		recheckCooldown,
	)
	if errors.Is(err, storage.ErrCooldown) {
		return actionDetailCooldown
	}

	if errors.Is(err, storage.ErrNotFound) {
		return actionDetailNoRun
	}

	if err != nil {
		return actionDetailRerun
	}

	s.logger.Info(
		"reran workflow run from comment",
		"repository", lt.Repository,
		"issue_number", lt.IssueNumber,
		"run_id", runID,
	)

	return ""
}

// recheckCooldown is how often the author can recheck a pull request.
const recheckCooldown = 5 * time.Minute

// rerunPullRequest reruns the workflow run stored for the pull request and
// releases the cooldown when GitHub rejects the rerun. A zero cooldown always
// reruns.
func (s *Server) rerunPullRequest(
	ctx context.Context,
	token string,
	repositoryID int64,
	repo string,
	issueNumber int64,
	cooldown time.Duration,
) (int64, error) {
	runID, err := s.store.TakePullRequestRun(
		ctx,
		repositoryID,
		issueNumber,
		cooldown,
	)
	if errors.Is(err, storage.ErrCooldown) {
		s.logger.Info(
			"skipping recheck: cooldown active",
			"repository", repo,
			"issue_number", issueNumber,
		)

		return 0, err
	}

	if errors.Is(err, storage.ErrNotFound) {
		s.logger.Info(
			"skipping recheck: no workflow run available",
			"repository", repo,
			"issue_number", issueNumber,
		)

		return 0, err
	}

	if err != nil {
		s.logger.Error(
			"taking pull request run for recheck",
			"error", err,
			"repository", repo,
			"issue_number", issueNumber,
		)

		return 0, err
	}

	if err := s.githubClient.RerunWorkflowRun(ctx, token, repo, runID); err != nil {
		if err := s.store.ReleasePullRequestRun(
			ctx,
			repositoryID,
			issueNumber,
		); err != nil {
			s.logger.Error(
				"releasing pull request run cooldown",
				"error", err,
				"repository", repo,
				"issue_number", issueNumber,
			)
		}

		s.logger.Warn(
			"failed to rerun workflow run after recheck",
			"error", err,
			"repository", repo,
			"run_id", runID,
		)

		return 0, err
	}

	return runID, nil
}

// closedByApp reports whether the app closed the pull request at closedAt.
func (s *Server) closedByApp(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
	closedAt *time.Time,
) (bool, error) {
	record, err := s.store.AutoClosedPullRequest(ctx, repositoryID, issueNumber)
	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return closedAt != nil && sameClosedAt(*closedAt, record.ClosedAt), nil
}

// actionError shows detail to the author on the action page.
func (s *Server) actionError(w http.ResponseWriter, lt linkToken, detail string) {
	page := actionPage{Error: detail}
	if lt.Repository != "" {
		page.PRURL = pullRequestURL(lt)
	}

	s.writeActionPage(w, http.StatusBadRequest, page)
}
