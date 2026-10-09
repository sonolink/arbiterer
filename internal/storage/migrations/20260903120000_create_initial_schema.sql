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

CREATE TYPE setup_comment_status AS ENUM ('unlinked', 'github_verified', 'linked', 'revoked', 'not_a_member');

CREATE TABLE setup_comments (
  repository_id BIGINT NOT NULL,
  issue_number BIGINT NOT NULL,
  comment_id BIGINT NOT NULL,
  status setup_comment_status NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (repository_id, issue_number)
);

CREATE TABLE auto_closed_pull_requests (
  repository_id BIGINT NOT NULL,
  issue_number BIGINT NOT NULL,
  run_id BIGINT NOT NULL,
  closed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (repository_id, issue_number)
);

-- +goose Down
DROP TABLE IF EXISTS github_discord_connections CASCADE;
DROP TABLE IF EXISTS discord_users CASCADE;
DROP TABLE IF EXISTS setup_comments CASCADE;
DROP TABLE IF EXISTS auto_closed_pull_requests CASCADE;
DROP TYPE IF EXISTS setup_comment_status;
