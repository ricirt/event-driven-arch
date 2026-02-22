package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ricirt/event-driven-arch/internal/config"
)

// Connect creates a pgxpool connection pool and verifies connectivity.
func Connect(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	poolCfg.MaxConns = cfg.DBMaxConns
	poolCfg.MinConns = cfg.DBMinConns

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

// Migrate runs all pending up-migrations from the migrations/ directory.
// It is idempotent: already-applied migrations are skipped.
func Migrate(databaseURL string) error {
	// golang-migrate's pgx/v5 driver expects the scheme "pgx5://".
	// Support both "postgres://" and "postgresql://" connection string forms.
	var rest string
	switch {
	case strings.HasPrefix(databaseURL, "postgresql://"):
		rest = databaseURL[len("postgresql://"):]
	case strings.HasPrefix(databaseURL, "postgres://"):
		rest = databaseURL[len("postgres://"):]
	default:
		rest = databaseURL
	}
	migrationURL := "pgx5://" + rest

	m, err := migrate.New("file://migrations", migrationURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}