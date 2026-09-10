-- +goose Up
DROP TABLE IF EXISTS links;
DROP TYPE IF EXISTS community;
DROP TYPE IF EXISTS forge;

CREATE TABLE pending_registrations (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  state_token TEXT COLLATE "C" NOT NULL UNIQUE,
  github_user_id TEXT COLLATE "C" NOT NULL,
  repository_id BIGINT NOT NULL,
  pr_number INT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ
);

CREATE INDEX idx_pending_registrations_cleanup
  ON pending_registrations (expires_at)
  WHERE used_at IS NULL;

CREATE TABLE github_users (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  github_user_id TEXT COLLATE "C" NOT NULL UNIQUE,
  created_at TIMESTAMPTZ GENERATED ALWAYS AS (uuid_extract_timestamp(id)) STORED
);

CREATE TABLE discord_users (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  discord_user_id TEXT COLLATE "C" NOT NULL UNIQUE,
  access_token TEXT NOT NULL,
  refresh_token TEXT NOT NULL,
  token_expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ GENERATED ALWAYS AS (uuid_extract_timestamp(id)) STORED
);

CREATE TABLE communities (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  installation_account_id BIGINT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ GENERATED ALWAYS AS (uuid_extract_timestamp(id)) STORED
);

CREATE TABLE repositories (
  repository_id BIGINT PRIMARY KEY,
  community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE connections_discord (
  id UUID PRIMARY KEY DEFAULT UUIDV7(),
  github_user_id UUID NOT NULL REFERENCES github_users(id),
  discord_user_id UUID NOT NULL REFERENCES discord_users(id),
  community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ GENERATED ALWAYS AS (uuid_extract_timestamp(id)) STORED,
  UNIQUE (github_user_id, community_id)
);

-- +goose Down
DROP TABLE IF EXISTS connections_discord;
DROP TABLE IF EXISTS repositories;
DROP TABLE IF EXISTS communities;
DROP TABLE IF EXISTS discord_users;
DROP TABLE IF EXISTS github_users;
DROP TABLE IF EXISTS pending_registrations;
