package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DiscordUser is a linked Discord account.
type DiscordUser struct {
	ID                    int64
	EncryptedAccessToken  []byte
	EncryptedRefreshToken []byte
	TokenExpiresAt        time.Time
}

// DiscordUserByConnection returns the Discord user connected to the given GitHub user
// within the given repository, or ErrNotFound when no user is connected.
func (s *Store) DiscordUserByConnection(
	ctx context.Context,
	githubUserID string,
	repositoryID int64,
) (*DiscordUser, error) {
	const query = `
		SELECT du.id, du.encrypted_access_token, du.encrypted_refresh_token, du.token_expires_at
		FROM github_discord_connections c
		JOIN discord_users du ON du.id = c.discord_user_id
		WHERE c.github_user_id = $1 AND c.repository_id = $2`

	var user DiscordUser

	err := s.pool.QueryRow(ctx, query, githubUserID, repositoryID).Scan(
		&user.ID,
		&user.EncryptedAccessToken,
		&user.EncryptedRefreshToken,
		&user.TokenExpiresAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("storage: discord user by connection: %w", err)
	}

	return &user, nil
}

// UpdateDiscordUserTokens replaces the stored credentials of a Discord account with the
// ones held by the user.
func (s *Store) UpdateDiscordUserTokens(ctx context.Context, user *DiscordUser) error {
	const query = `
		UPDATE discord_users
		SET encrypted_access_token = $1,
			encrypted_refresh_token = $2,
			token_expires_at = $3
		WHERE id = $4
		`

	tag, err := s.pool.Exec(
		ctx,
		query,
		user.EncryptedAccessToken,
		user.EncryptedRefreshToken,
		user.TokenExpiresAt,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("storage: update discord tokens: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// LinkGitHubDiscord connects a GitHub user to a Discord account.
func (s *Store) LinkGitHubDiscord(
	ctx context.Context,
	githubUserID string,
	repositoryID int64,
	user *DiscordUser,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("storage: link github discord: %w", err)
	}
	defer tx.Rollback(ctx)

	const upsertUserQuery = `
		INSERT INTO discord_users (id, encrypted_access_token, encrypted_refresh_token, token_expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET encrypted_access_token = EXCLUDED.encrypted_access_token,
			encrypted_refresh_token = EXCLUDED.encrypted_refresh_token,
			token_expires_at = EXCLUDED.token_expires_at
	`
	if _, err := tx.Exec(
		ctx,
		upsertUserQuery,
		user.ID,
		user.EncryptedAccessToken,
		user.EncryptedRefreshToken,
		user.TokenExpiresAt,
	); err != nil {
		return fmt.Errorf("storage: link github discord: %w", err)
	}

	const upsertConnectionQuery = `
		INSERT INTO github_discord_connections (github_user_id, discord_user_id, repository_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (github_user_id, repository_id) DO UPDATE
		SET discord_user_id = EXCLUDED.discord_user_id
	`
	if _, err := tx.Exec(ctx, upsertConnectionQuery, githubUserID, user.ID, repositoryID); err != nil {
		return fmt.Errorf("storage: link github discord: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("storage: link github discord: %w", err)
	}

	return nil
}
