-- +goose Up
CREATE TABLE pull_request_runs (
  repository_id BIGINT NOT NULL,
  issue_number BIGINT NOT NULL,
  run_id BIGINT NOT NULL,
  last_recheck_at TIMESTAMPTZ,
  PRIMARY KEY (repository_id, issue_number)
);

-- +goose Down
DROP TABLE IF EXISTS pull_request_runs;
