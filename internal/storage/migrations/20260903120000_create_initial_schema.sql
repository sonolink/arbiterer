-- +goose Up
CREATE TABLE discord_users (
  id BIGINT PRIMARY KEY,
  encrypted_access_token BYTEA NOT NULL,
  encrypted_refresh_token BYTEA NOT NULL,
  token_expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE github_discord_connections (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  github_user_id TEXT COLLATE "C" NOT NULL,
  discord_user_id BIGINT NOT NULL REFERENCES discord_users(id) ON DELETE CASCADE,
  repository_id BIGINT NOT NULL,
  UNIQUE (github_user_id, repository_id),
  UNIQUE (discord_user_id, repository_id)
);

-- +goose Down
DROP TABLE IF EXISTS github_discord_connections CASCADE;
DROP TABLE IF EXISTS discord_users CASCADE;
