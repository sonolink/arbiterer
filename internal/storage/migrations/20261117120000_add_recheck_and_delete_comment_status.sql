-- +goose NO TRANSACTION

-- +goose Up
ALTER TYPE setup_comment_status ADD VALUE IF NOT EXISTS 'rechecking';
ALTER TYPE setup_comment_status ADD VALUE IF NOT EXISTS 'deleted';

-- +goose Down
-- Postgres cannot drop enum values; the extra values are harmless.
SELECT 1;
