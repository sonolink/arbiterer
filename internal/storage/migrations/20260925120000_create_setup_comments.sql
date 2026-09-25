-- +goose Up
CREATE TABLE setup_comments (
  repository_id BIGINT NOT NULL,
  pull_request_number BIGINT NOT NULL,
  comment_id BIGINT NOT NULL,
  status TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (repository_id, pull_request_number)
);

-- +goose Down
DROP TABLE IF EXISTS setup_comments;
