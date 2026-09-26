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

CREATE TABLE setup_comments (
  repository_id BIGINT NOT NULL,
  pull_request_number BIGINT NOT NULL,
  comment_id BIGINT NOT NULL,
  status TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (repository_id, pull_request_number)
);

-- +goose Down
DROP TABLE IF EXISTS github_discord_connections CASCADE;
DROP TABLE IF EXISTS discord_users CASCADE;
DROP TABLE IF EXISTS setup_comments CASCADE;
