package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sonolink/arbiterer/internal/github"
	"github.com/sonolink/arbiterer/internal/storage"
)

// setupStep is one step a contributor has to complete before the check passes.
type setupStep struct {
	text string
	done bool
}

// commentStatus is the status the setup comment reports to the PR author.
type commentStatus string

const (
	// commentGitHubVerified marks a contributor who signed in with GitHub but
	// has not yet authorized Discord. It is never served by resolve.
	commentGitHubVerified commentStatus = "github_verified"

	commentUnlinked    commentStatus = "unlinked"
	commentLinked      commentStatus = "linked"
	commentRevoked     commentStatus = "revoked"
	commentRulesFailed commentStatus = "rules_failed"
)

// setupSteps lists the steps a contributor must complete and links the next open step to linkURL.
func setupSteps(status commentStatus, linkURL string) []setupStep {
	signInGitHub := setupStep{text: "Sign in with GitHub"}
	signInDiscord := setupStep{text: "Sign in with Discord"}

	switch status {
	case commentLinked:
		// Nothing left to do; the comment confirms the link or clears an earlier ask.
		signInGitHub.done = true
		signInDiscord.done = true
	case commentGitHubVerified:
		// Resumes at the Discord step via the cookie set in the GitHub callback.
		signInGitHub.done = true
		signInDiscord.text = fmt.Sprintf("[%s](%s)", signInDiscord.text, linkURL)
	default:
		// commentUnlinked, commentRevoked, and anything new start at GitHub.
		signInGitHub.text = fmt.Sprintf("[%s](%s)", signInGitHub.text, linkURL)
	}

	return []setupStep{signInGitHub, signInDiscord}
}

// formatCommentBody renders the setup comment for a contributor in the given
// status, with completed steps struck through.
func formatCommentBody(author string, status commentStatus, linkURL string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "@%s, ", author)

	switch status {
	case commentLinked:
		b.WriteString("your GitHub and Discord accounts are linked.")
	case commentRevoked:
		b.WriteString("your Discord link has expired or was revoked. Please follow these steps to link it again:")
	case commentRulesFailed:
		b.WriteString("your pull request was closed because you don't satisfy the pre-defined rules.")

		if linkURL != "" {
			fmt.Fprintf(&b, " [See more here](%s).", linkURL)
		}

		return b.String()
	default:
		b.WriteString("please follow these steps to continue:")
	}

	b.WriteString("\n")

	for i, step := range setupSteps(status, linkURL) {
		text := step.text
		if step.done {
			text = "~~" + text + "~~"
		}

		fmt.Fprintf(&b, "\n%d. %s", i+1, text)
	}

	return b.String()
}

// setupCommentNeedsWrite reports whether the setup comment has to be posted or
// updated. A linked contributor needs no comment unless an earlier one asked
// them to act or has opted in to a comment confirming the link, and a comment
// without a link to refresh only changes with the status.
func setupCommentNeedsWrite(
	stored *storage.SetupComment,
	status commentStatus,
	linkURL string,
	commentOnLinked bool,
) bool {
	if stored == nil {
		return status != commentLinked || commentOnLinked
	}

	return stored.Status != string(status) || linkURL != ""
}

// syncSetupComment reconciles the comment telling a contributor what to do.
func (s *Server) syncSetupComment(
	ctx context.Context,
	repositoryID int64,
	repo string,
	issueNumber int64,
	status commentStatus,
	linkURL string,
	commentOnLinked bool,
) error {
	if issueNumber == 0 {
		return nil
	}

	stored, err := s.store.SetupCommentByPullRequest(ctx, repositoryID, issueNumber)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("looking up setup comment: %w", err)
	}

	if !setupCommentNeedsWrite(stored, status, linkURL, commentOnLinked) {
		return nil
	}

	token, err := s.githubClient.CreateInstallationToken(ctx, repositoryID, repo)
	if err != nil {
		return fmt.Errorf("creating installation token: %w", err)
	}

	author, err := s.githubClient.FetchIssueAuthor(ctx, token, repo, issueNumber)
	if err != nil {
		return fmt.Errorf("fetching issue author (%d): %w", issueNumber, err)
	}

	body := formatCommentBody(author.Login, status, linkURL)
	comment := &storage.SetupComment{
		RepositoryID: repositoryID,
		IssueNumber:  issueNumber,
		Status:       string(status),
	}

	if stored != nil {
		comment.CommentID = stored.CommentID

		err := s.githubClient.UpdateComment(ctx, token, repo, stored.CommentID, body)

		var apiErr *github.APIError
		commentDeleted := errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound

		switch {
		case err == nil, commentDeleted && status == commentLinked:
			return s.store.UpsertSetupComment(ctx, comment)
		case !commentDeleted:
			return fmt.Errorf("updating comment: %w", err)
		}
	}

	comment.CommentID, err = s.githubClient.CreateComment(ctx, token, repo, issueNumber, body)
	if err != nil {
		return fmt.Errorf("creating comment: %w", err)
	}

	return s.store.UpsertSetupComment(ctx, comment)
}

// logSetupCommentError logs a failed comment sync. A missing app installation
// is the repository's setup to fix, not a server fault, so it only warns.
func (s *Server) logSetupCommentError(err error) {
	if errors.Is(err, github.ErrAppNotInstalled) {
		s.logger.Warn("skipping setup comment", "error", err)
		return
	}

	s.logger.Error("syncing setup comment", "error", err)
}

type closeRequest struct {
	IssueNumber int64 `json:"issue_number"` // GitHub's API addresses PRs as issues.
	RulesFailed bool  `json:"rules_failed,omitempty"`
}

type closeResponse struct {
	Closed   bool `json:"closed"`
	Recorded bool `json:"recorded"`
}

// handleClosePullRequest answers POST /v1/pulls/close.
func (s *Server) handleClosePullRequest(w http.ResponseWriter, r *http.Request) {
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

	if pr.State != "open" {
		// Already closed or merged: not the app's close to record or undo.
		if req.RulesFailed && !pr.Merged {
			s.refreshRulesFailed(ctx, claims, req.IssueNumber)
		}

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

	if req.RulesFailed {
		s.notifyRulesFailed(ctx, claims, req.IssueNumber)
	}

	s.writeJSON(w, http.StatusOK, closeResponse{Closed: true, Recorded: recorded})
}

// notifyRulesFailed reconciles the setup comment to explain that failed rules
// closed the pull request.
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

// refreshRulesFailed re-points the rules notice at the latest run.
func (s *Server) refreshRulesFailed(ctx context.Context, claims *github.Claims, issueNumber int64) {
	if _, err := s.store.AutoClosedPullRequest(ctx, claims.RepositoryID, issueNumber); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			s.logger.Error(
				"reading auto-closed record",
				"error", err,
				"issue_number", issueNumber,
			)
		}

		return
	}

	s.notifyRulesFailed(ctx, claims, issueNumber)
}

func workflowRunURL(claims *github.Claims) string {
	if claims.RunID == 0 {
		return ""
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

// handleOpenPullRequest answers POST /v1/pulls/open: opens a pull request the app auto-closed.
func (s *Server) handleOpenPullRequest(w http.ResponseWriter, r *http.Request) {
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
