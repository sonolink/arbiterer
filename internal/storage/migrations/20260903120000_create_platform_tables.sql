-- +goose Up
DROP TABLE IF EXISTS links;
DROP TYPE IF EXISTS community;
DROP TYPE IF EXISTS forge;

CREATE TABLE pending_registrations (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  state_token TEXT COLLATE "C" NOT NULL UNIQUE,
  github_user_id TEXT COLLATE "C" NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ
);

CREATE INDEX idx_pending_registrations_cleanup
  ON pending_registrations (expires_at)
  WHERE used_at IS NULL;

CREATE TABLE discord_users (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  discord_user_id TEXT COLLATE "C" NOT NULL UNIQUE,
  access_token TEXT NOT NULL,
  refresh_token TEXT NOT NULL,
  token_expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ GENERATED ALWAYS AS (uuid_extract_timestamp(id)) VIRTUAL
);

CREATE TABLE github_discord_connections (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  github_user_id TEXT COLLATE "C" NOT NULL,
  discord_user_id UUID NOT NULL REFERENCES discord_users(id),
  repository_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ GENERATED ALWAYS AS (uuid_extract_timestamp(id)) VIRTUAL,
  UNIQUE (github_user_id, repository_id)
);

-- +goose Down
DROP TABLE IF EXISTS connections_discord;
DROP TABLE IF EXISTS discord_users;
DROP TABLE IF EXISTS pending_registrations;
