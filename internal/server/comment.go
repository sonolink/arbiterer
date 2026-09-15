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

// commentBody assembles a setup comment's body: the marker, followed by one
// or more lines.
type commentBody struct {
	lines []string
}

// addLine appends a line to the comment, optionally mentioning author first.
func (b *commentBody) addLine(author, text string) *commentBody {
	if author != "" {
		text = "@" + author + ", " + text
	}

	b.lines = append(b.lines, text)

	return b
}

// addStruckLine appends a previous line rendered as struck-through, keeping
// its original mention since editing a comment doesn't notify anyone.
func (b *commentBody) addStruckLine(mention, text string) *commentBody {
	b.lines = append(b.lines, mention+"~~"+text+"~~")
	return b
}

func (b *commentBody) String() string {
	return commentMarker + "\n\n" + strings.Join(b.lines, "\n\n")
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
	issueNumber int64,
	status resolveStatus,
	setupURL string,
	guildChecked bool,
) error {
	existing, err := s.githubApp.FindComment(ctx, repo, issueNumber, commentMarker)
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

	var b commentBody

	switch {
	case existing == nil:
		// A fresh comment, so mention them to actually get their attention.
		author, err := s.githubApp.IssueAuthorLogin(ctx, repo, issueNumber)
		if err != nil {
			return fmt.Errorf("fetching issue author: %w", err)
		}

		b.addLine(author, message)
	case status == statusLinked:
		author, err := s.githubApp.IssueAuthorLogin(ctx, repo, issueNumber)
		if err != nil {
			return fmt.Errorf("fetching issue author: %w", err)
		}

		mention := "@" + author + ", "
		b.addStruckLine(mention, previousMessage(existing.Body, mention)).addLine("", message)
	default:
		b.addLine("", message)
	}

	body := b.String()

	if existing == nil {
		if err := s.githubApp.CreateComment(ctx, repo, issueNumber, body); err != nil {
			return fmt.Errorf("creating comment: %w", err)
		}

		return nil
	}

	if err := s.githubApp.UpdateComment(ctx, repo, existing.ID, body); err != nil {
		return fmt.Errorf("updating comment: %w", err)
	}

	return nil
}
