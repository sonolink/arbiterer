package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SetupComment is the comment the app posted on a pull request telling its
// author how to link, along with the status it last reported.
type SetupComment struct {
	RepositoryID int64
	IssueNumber  int64
	CommentID    int64
	Status       string
}

// SetupCommentByPullRequest returns the setup comment posted on the given pull
// request, or ErrNotFound when none has been posted.
func (s *Store) SetupCommentByPullRequest(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
) (*SetupComment, error) {
	const query = `
		SELECT comment_id, status
		FROM setup_comments
		WHERE repository_id = $1 AND issue_number = $2`

	comment := SetupComment{
		RepositoryID: repositoryID,
		IssueNumber:  issueNumber,
	}

	err := s.pool.QueryRow(ctx, query, repositoryID, issueNumber).Scan(
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

// UpsertSetupComment stores the setup comment of a pull request, replacing any
// previously stored one.
func (s *Store) UpsertSetupComment(ctx context.Context, comment *SetupComment) error {
	const query = `
		INSERT INTO setup_comments (repository_id, issue_number, comment_id, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (repository_id, issue_number) DO UPDATE
		SET comment_id = EXCLUDED.comment_id,
			status = EXCLUDED.status,
			updated_at = NOW()
	`

	if _, err := s.pool.Exec(
		ctx,
		query,
		comment.RepositoryID,
		comment.IssueNumber,
		comment.CommentID,
		comment.Status,
	); err != nil {
		return fmt.Errorf("storage: upsert setup comment: %w", err)
	}

	return nil
}

// SetSetupCommentRerunJob stores the job to re-run once the author links,
// replacing any previously stored one.
func (s *Store) SetSetupCommentRerunJob(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
	jobID int64,
) error {
	const query = `
		UPDATE setup_comments
		SET rerun_job_id = $3,
			updated_at = NOW()
		WHERE repository_id = $1 AND issue_number = $2`

	if _, err := s.pool.Exec(ctx, query, repositoryID, issueNumber, jobID); err != nil {
		return fmt.Errorf("storage: set setup comment rerun job: %w", err)
	}

	return nil
}

// TakeSetupCommentRerunJob clears and returns the job stored for the pull
// request, or ErrNotFound when none is stored.
func (s *Store) TakeSetupCommentRerunJob(ctx context.Context, repositoryID, issueNumber int64) (int64, error) {
	const query = `
		UPDATE setup_comments
		SET rerun_job_id = NULL,
			updated_at = NOW()
		WHERE repository_id = $1 AND issue_number = $2 AND rerun_job_id IS NOT NULL
		RETURNING rerun_job_id`

	var jobID int64
	err := s.pool.QueryRow(ctx, query, repositoryID, issueNumber).Scan(&jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}

	if err != nil {
		return 0, fmt.Errorf("storage: take setup comment rerun job: %w", err)
	}

	return jobID, nil
}

// ClearSetupCommentRerunJob forgets the job stored for the pull request.
func (s *Store) ClearSetupCommentRerunJob(ctx context.Context, repositoryID, issueNumber int64) error {
	const query = `
		UPDATE setup_comments
		SET rerun_job_id = NULL,
			updated_at = NOW()
		WHERE repository_id = $1 AND issue_number = $2`

	if _, err := s.pool.Exec(ctx, query, repositoryID, issueNumber); err != nil {
		return fmt.Errorf("storage: clear setup comment rerun job: %w", err)
	}

	return nil
}
