package server

import (
	"context"
	"fmt"
	"strings"
)

// commentMarker is hidden in the comment body so later runs can find and edit
// it instead of posting a new one every time. HTML comments don't render, so
// it's never shown to the user.
const commentMarker = "<!-- arbiterer-setup-message -->"

// commentMessage describes what a contributor needs to do next for a given
// resolve status. guildChecked reports whether this resolve request checked
// server membership at all, since a "linked" contributor who was also
// confirmed as a member should hear about that too.
func commentMessage(status resolveStatus, setupURL string, guildChecked bool) string {
	switch status {
	case statusLinked:
		if guildChecked {
			return "Your GitHub account is linked to your Discord account and you are also a member of the required server. No further action is needed."
		}

		return "Your GitHub account is linked to your Discord account. No further action is needed."
	case statusUnlinked:
		return fmt.Sprintf("Please [link your Discord account](%s) to your GitHub account.", setupURL)
	case statusRevoked:
		return fmt.Sprintf(
			"Your Discord account link has expired or was revoked. Please [re-link your account](%s).",
			setupURL,
		)
	case statusNotAMember:
		return "Your GitHub account is linked, but you are not in the required Discord server. Please join the server."
	default:
		return fmt.Sprintf("Unable to confirm your Discord account link. Status: %s", status)
	}
}

// previousMessage extracts the message text from a previous marked comment's
// body, stripping the marker and the leading mention so it can be re-wrapped
// under a fresh, unstruck mention line.
func previousMessage(body, mention string) string {
	text := strings.TrimSpace(strings.ReplaceAll(body, commentMarker, ""))
	return strings.TrimPrefix(text, mention)
}

// syncSetupComment reconciles the comment telling a contributor what they
// need to do to link their Discord account, on the given issue or pull
// request. Once linked, a previous comment is edited to say so rather than
// left showing stale instructions; if a contributor was already linked
// before ever seeing that comment, nothing is posted. It is best-effort: the
// action's outputs already carry the status, so a failure here is logged
// rather than failing the whole request.
func (s *Server) syncSetupComment(
	ctx context.Context,
	repo string,
	pullRequestNumber int64,
	status resolveStatus,
	setupURL string,
	guildChecked bool,
) error {
	existing, err := s.githubClient.FindComment(ctx, repo, pullRequestNumber, commentMarker)
	if err != nil {
		return fmt.Errorf("finding comment: %w", err)
	}

	message := commentMessage(status, setupURL, guildChecked)

	if status == statusLinked && (existing == nil || strings.Contains(existing.Body, message)) {
		// Nothing to tell them: either they were never shown a setup comment,
		// or this comment already reflects the linked state - editing again
		// would strike through an already-struck message.
		return nil
	}

	var body string

	switch {
	case existing == nil:
		author, err := s.githubClient.PullRequestAuthorLogin(ctx, repo, pullRequestNumber)
		if err != nil {
			return fmt.Errorf("fetching issue author: %w", err)
		}

		body = commentMarker + "\n\n@" + author + ", " + message
	case status == statusLinked:
		// Keep the original mention on the struck-through line; editing a
		// comment doesn't notify anyone, so the new line doesn't need one.
		author, err := s.githubClient.PullRequestAuthorLogin(ctx, repo, pullRequestNumber)
		if err != nil {
			return fmt.Errorf("fetching issue author: %w", err)
		}

		mention := "@" + author + ", "
		body = commentMarker + "\n\n" + mention + "~~" + previousMessage(existing.Body, mention) + "~~\n\n" + message
	default:
		body = commentMarker + "\n\n" + message
	}

	if existing == nil {
		if err := s.githubClient.CreateComment(ctx, repo, pullRequestNumber, body); err != nil {
			return fmt.Errorf("creating comment: %w", err)
		}

		return nil
	}

	if err := s.githubClient.UpdateComment(ctx, repo, existing.ID, body); err != nil {
		return fmt.Errorf("updating comment: %w", err)
	}

	return nil
}
