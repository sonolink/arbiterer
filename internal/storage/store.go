package storage

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound is returned when a query matches no rows.
var ErrNotFound = errors.New("storage: not found")

// Store enables interaction with the postgres database.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore opens a connection pool to the given DSN and makes sure the database
// is reachable before returning.
func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("storage: ping: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Migrate applies any pending migrations to the database.
func (s *Store) Migrate(ctx context.Context) error {
	goose.SetBaseFS(migrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("storage: goose dialect: %w", err)
	}

	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("storage: migrate: %w", err)
	}

	return nil
}

// Close releases the database connection pool.
func (s *Store) Close() {
	s.pool.Close()
}
