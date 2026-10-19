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

package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// UpsertPullRequestRun stores the latest workflow run that verified the pull
// request, replacing any previously stored one.
func (s *Store) UpsertPullRequestRun(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
	runID int64,
) error {
	const query = `
		INSERT INTO pull_request_runs (repository_id, issue_number, run_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (repository_id, issue_number) DO UPDATE
		SET run_id = EXCLUDED.run_id`

	if _, err := s.pool.Exec(ctx, query, repositoryID, issueNumber, runID); err != nil {
		return fmt.Errorf("storage: upsert pull request run: %w", err)
	}

	return nil
}

// TakePullRequestRun returns the latest workflow run stored for the pull
// request and records that a recheck started. It returns ErrNotFound when no
// run is stored and ErrCooldown when the cooldown has not elapsed.
func (s *Store) TakePullRequestRun(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
	cooldown time.Duration,
) (int64, error) {
	const query = `
		UPDATE pull_request_runs
		SET last_recheck_at = NOW()
		WHERE repository_id = $1
			AND issue_number = $2
			AND (
				last_recheck_at IS NULL
				OR last_recheck_at <= NOW() - ($3::double precision * INTERVAL '1 second')
			)
		RETURNING run_id`

	var runID int64

	err := s.pool.QueryRow(
		ctx,
		query,
		repositoryID,
		issueNumber,
		cooldown.Seconds(),
	).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool

		existsQuery := `
			SELECT EXISTS (
				SELECT 1
				FROM pull_request_runs
				WHERE repository_id = $1 AND issue_number = $2
			)`

		if err := s.pool.QueryRow(ctx, existsQuery, repositoryID, issueNumber).Scan(&exists); err != nil {
			return 0, fmt.Errorf("storage: checking pull request run: %w", err)
		}

		if exists {
			return 0, ErrCooldown
		}

		return 0, ErrNotFound
	}

	if err != nil {
		return 0, fmt.Errorf("storage: take pull request run: %w", err)
	}

	return runID, nil
}

// ReleasePullRequestRun clears the recheck cooldown so a failed rerun can be
// retried immediately.
func (s *Store) ReleasePullRequestRun(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
) error {
	const query = `
		UPDATE pull_request_runs
		SET last_recheck_at = NULL
		WHERE repository_id = $1 AND issue_number = $2`

	if _, err := s.pool.Exec(ctx, query, repositoryID, issueNumber); err != nil {
		return fmt.Errorf("storage: release pull request run: %w", err)
	}

	return nil
}
