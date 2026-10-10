package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AutoClosedPullRequest records that the app closed a pull request and has not opened.
type AutoClosedPullRequest struct {
	RepositoryID int64
	IssueNumber  int64
	ClosedAt     time.Time
}

// UpsertAutoClosedPullRequest records that the app closed the pull request at closedAt.
func (s *Store) UpsertAutoClosedPullRequest(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
	closedAt time.Time,
) error {
	const query = `
		INSERT INTO auto_closed_pull_requests (repository_id, issue_number, closed_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (repository_id, issue_number) DO UPDATE
		SET closed_at = EXCLUDED.closed_at`

	if _, err := s.pool.Exec(ctx, query, repositoryID, issueNumber, closedAt); err != nil {
		return fmt.Errorf("storage: upsert auto closed pull request: %w", err)
	}

	return nil
}

// DeleteAutoClosedPullRequest forgets that the app closed the pull request.
func (s *Store) DeleteAutoClosedPullRequest(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
) error {
	const query = `
		DELETE FROM auto_closed_pull_requests
		WHERE repository_id = $1 AND issue_number = $2`

	if _, err := s.pool.Exec(ctx, query, repositoryID, issueNumber); err != nil {
		return fmt.Errorf("storage: delete auto closed pull request: %w", err)
	}

	return nil
}

// AutoClosedPullRequest returns the record for the pull request, or ErrNotFound
// when the app has not closed it.
func (s *Store) AutoClosedPullRequest(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
) (*AutoClosedPullRequest, error) {
	const query = `
		SELECT closed_at
		FROM auto_closed_pull_requests
		WHERE repository_id = $1 AND issue_number = $2`

	record := AutoClosedPullRequest{
		RepositoryID: repositoryID,
		IssueNumber:  issueNumber,
	}

	err := s.pool.QueryRow(ctx, query, repositoryID, issueNumber).Scan(&record.ClosedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("storage: auto closed pull request: %w", err)
	}

	return &record, nil
}
