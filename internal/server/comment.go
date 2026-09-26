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

// commentStatus is the status the setup comment reports to a contributor.
type commentStatus string

const (
	// commentGitHubVerified marks a contributor who signed in with GitHub but
	// has not yet authorized Discord. It is never served by resolve.
	commentGitHubVerified commentStatus = "github_verified"

	commentLinked     commentStatus = "linked"
	commentRevoked    commentStatus = "revoked"
	commentNotAMember commentStatus = "not_a_member"
)

// setupSteps lists the steps for linking a contributor's accounts, linking the
// next step to take with linkURL.
func setupSteps(status commentStatus, linkURL string) []setupStep {
	signInGitHub := setupStep{text: "Sign in with GitHub"}
	signInDiscord := setupStep{text: "Sign in with Discord"}

	switch status {
	case commentLinked:
		signInGitHub.done = true
		signInDiscord.done = true
	case commentGitHubVerified, commentRevoked:
		signInGitHub.done = true
		signInDiscord.text = fmt.Sprintf("[%s](%s)", signInDiscord.text, linkURL)
	default:
		signInGitHub.text = fmt.Sprintf("[%s](%s)", signInGitHub.text, linkURL)
	}

	return []setupStep{signInGitHub, signInDiscord}
}

// formatCommentBody renders the setup comment for a contributor in the given
// status, with completed steps struck through.
func formatCommentBody(author string, status commentStatus, linkURL string) string {
	var b strings.Builder

	b.WriteString("@" + author + ", ")

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
// them to act, and a comment without a link to refresh only changes with the status.
func setupCommentNeedsWrite(stored *storage.SetupComment, status commentStatus, linkURL string) bool {
	if stored == nil {
		return status != commentLinked
	}

	return stored.Status != string(status) || linkURL != ""
}

// syncSetupComment reconciles the comment telling a contributor what they
// need to do.
func (s *Server) syncSetupComment(
	ctx context.Context,
	repositoryID int64,
	repo string,
	pullRequestNumber int64,
	status commentStatus,
	linkURL string,
) error {
	// The comment only covers linking. Whatever else maintainers require, such
	// as server membership, is theirs to report.
	if status == commentNotAMember {
		status = commentLinked
	}

	stored, err := s.store.SetupCommentByPullRequest(ctx, repositoryID, pullRequestNumber)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("looking up setup comment: %w", err)
	}

	if !setupCommentNeedsWrite(stored, status, linkURL) {
		return nil
	}

	token, err := s.githubClient.CreateInstallationToken(ctx, repositoryID, repo)
	if err != nil {
		return fmt.Errorf("creating installation token: %w", err)
	}

	author, err := s.githubClient.FetchPullRequestAuthorLogin(ctx, token, repo, pullRequestNumber)
	if err != nil {
		return fmt.Errorf("fetching pull request author: %w", err)
	}

	body := formatCommentBody(author, status, linkURL)
	comment := &storage.SetupComment{
		RepositoryID:      repositoryID,
		PullRequestNumber: pullRequestNumber,
		Status:            string(status),
	}

	if stored != nil {
		comment.CommentID = stored.CommentID

		err := s.githubClient.UpdateComment(ctx, token, repo, stored.CommentID, body)

		var apiErr *github.APIError
		commentDeleted := errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound

		switch {
		case err == nil, commentDeleted && status == commentLinked:
			return s.store.SaveSetupComment(ctx, comment)
		case !commentDeleted:
			return fmt.Errorf("updating comment: %w", err)
		}
	}

	comment.CommentID, err = s.githubClient.CreateComment(ctx, token, repo, pullRequestNumber, body)
	if err != nil {
		return fmt.Errorf("creating comment: %w", err)
	}

	return s.store.SaveSetupComment(ctx, comment)
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
