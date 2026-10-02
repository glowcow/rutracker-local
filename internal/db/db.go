package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // sql.Open("pgx", ...) for goose
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect returns a pgxpool tuned for moderate concurrency.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse pg url: %w", err)
	}
	// Defaults are fine for homelab single-user; tune via env vars later if needed.
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

// migrateLockID keys the cross-process advisory lock serialising MigrateUp.
// Arbitrary but stable — must never collide with another advisory-lock user
// on the same database (we have none).
const migrateLockID int64 = 0x72747261636b6572 // "rtracker"

// MigrateUp applies pending goose migrations from the embedded FS, serialised
// via a pg advisory lock (goose's UpContext doesn't lock, so two booting replicas
// would race the same DDL and one crash-loops on "already exists"). The lock is
// session-scoped on one dedicated conn; deferred Close releases it even on error.
func MigrateUp(ctx context.Context, url string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open sql: %w", err)
	}
	defer db.Close()

	lockConn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate lock conn: %w", err)
	}
	defer lockConn.Close()
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrateLockID); err != nil {
		return fmt.Errorf("acquire migrate lock: %w", err)
	}
	defer func() {
		_, _ = lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, migrateLockID)
	}()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	return goose.UpContext(ctx, db, "migrations")
}

// MigrateStatus prints the migration status table to stdout.
func MigrateStatus(ctx context.Context, url string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open sql: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	return goose.StatusContext(ctx, db, "migrations")
}
