package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SetupComment is the comment the app posted on a pull request telling its
// author how to link their accounts, along with the status it last reported.
type SetupComment struct {
	RepositoryID      int64
	PullRequestNumber int64
	CommentID         int64
	Status            string
}

// SetupCommentByPullRequest returns the setup comment posted on the given pull
// request, or ErrNotFound when none has been posted.
func (s *Store) SetupCommentByPullRequest(
	ctx context.Context,
	repositoryID int64,
	pullRequestNumber int64,
) (*SetupComment, error) {
	const query = `
		SELECT comment_id, status
		FROM setup_comments
		WHERE repository_id = $1 AND pull_request_number = $2`

	comment := SetupComment{
		RepositoryID:      repositoryID,
		PullRequestNumber: pullRequestNumber,
	}

	err := s.pool.QueryRow(ctx, query, repositoryID, pullRequestNumber).Scan(
		&comment.CommentID,
		&comment.Status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("storage: setup comment by pull request: %w", err)
	}

	return &comment, nil
}

// SaveSetupComment stores the setup comment of a pull request, replacing any
// previously stored one.
func (s *Store) SaveSetupComment(ctx context.Context, comment *SetupComment) error {
	const query = `
		INSERT INTO setup_comments (repository_id, pull_request_number, comment_id, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (repository_id, pull_request_number) DO UPDATE
		SET comment_id = EXCLUDED.comment_id,
			status = EXCLUDED.status,
			updated_at = NOW()
	`

	if _, err := s.pool.Exec(
		ctx,
		query,
		comment.RepositoryID,
		comment.PullRequestNumber,
		comment.CommentID,
		comment.Status,
	); err != nil {
		return fmt.Errorf("storage: save setup comment: %w", err)
	}

	return nil
}
