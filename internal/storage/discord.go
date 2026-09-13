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
