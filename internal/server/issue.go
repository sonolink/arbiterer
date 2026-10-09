package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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

	commentUnlinked commentStatus = "unlinked"
	commentLinked   commentStatus = "linked"
	commentRevoked  commentStatus = "revoked"
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

func (s *Server) ClosePullRequest(w http.ResponseWriter, r *http.Request) {
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

	token, err := s.githubClient.CreateInstallationToken(ctx, claims.RepositoryID, claims.Repository)
	if err != nil {
		s.logger.Error("creating installation token", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	err = s.githubClient.ClosePullRequest(ctx, token, claims.Repository, req.IssueNumber)
	if err != nil {
		s.logger.Error("closing pull request", "error", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]bool{"closed": true})
}
