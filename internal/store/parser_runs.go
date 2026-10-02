package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ParserRun is the durable record of one parse invocation. Inserted at
// start with `Status="running"`, completed (or marked failed) at finish.
// The API reads the latest row into prometheus gauges so dashboards can
// answer "when did the last full ingest finish, and what did it do".
type ParserRun struct {
	ID           int64
	StartedAt    time.Time
	FinishedAt   *time.Time
	Source       string
	RowsInserted int64
	RowsSwept    int64
	Sweep        bool
	Status       string // "running" | "succeeded" | "failed"
}

const (
	ParserRunRunning   = "running"
	ParserRunSucceeded = "succeeded"
	ParserRunFailed    = "failed"
)

// StartParserRun inserts a row in `running` state and returns its id. The
// id is then passed back to FinishParserRun / FailParserRun.
func StartParserRun(ctx context.Context, pool *pgxpool.Pool, source string, sweep bool) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO parser_runs (source, sweep, status)
		VALUES ($1, $2, $3)
		RETURNING id
	`, source, sweep, ParserRunRunning).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("start parser run: %w", err)
	}
	return id, nil
}

// FinishParserRun stamps the row with the ingest results and `succeeded`.
func FinishParserRun(ctx context.Context, pool *pgxpool.Pool, id, rowsInserted, rowsSwept int64) error {
	_, err := pool.Exec(ctx, `
		UPDATE parser_runs
		SET finished_at = NOW(),
		    rows_inserted = $2,
		    rows_swept = $3,
		    status = $4
		WHERE id = $1
	`, id, rowsInserted, rowsSwept, ParserRunSucceeded)
	if err != nil {
		return fmt.Errorf("finish parser run %d: %w", id, err)
	}
	return nil
}

// FailParserRun stamps the row with `failed`. Caller logs the underlying
// error separately — we don't persist it (keeps the schema lean; logs are
// the source of truth for diagnostics).
func FailParserRun(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	_, err := pool.Exec(ctx, `
		UPDATE parser_runs
		SET finished_at = NOW(), status = $2
		WHERE id = $1
	`, id, ParserRunFailed)
	if err != nil {
		return fmt.Errorf("fail parser run %d: %w", id, err)
	}
	return nil
}

// LatestParserRun returns the most recent row by started_at. Used by the
// API to populate the parser-side prometheus gauges. Returns ErrNotFound
// when the table is empty (fresh deploy, no parse has run yet).
func LatestParserRun(ctx context.Context, pool *pgxpool.Pool) (ParserRun, error) {
	var r ParserRun
	err := pool.QueryRow(ctx, `
		SELECT id, started_at, finished_at, source, rows_inserted, rows_swept, sweep, status
		FROM parser_runs
		ORDER BY started_at DESC
		LIMIT 1
	`).Scan(
		&r.ID, &r.StartedAt, &r.FinishedAt, &r.Source,
		&r.RowsInserted, &r.RowsSwept, &r.Sweep, &r.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ParserRun{}, ErrNotFound
		}
		return ParserRun{}, fmt.Errorf("latest parser run: %w", err)
	}
	return r, nil
}

// LatestSucceededParserRun returns the most recent succeeded run — the API's
// cache-version source. It keeps pointing at the last good sweep while a parse
// runs or after a failure, so cached snapshots stay valid. ErrNotFound if none.
func LatestSucceededParserRun(ctx context.Context, pool *pgxpool.Pool) (ParserRun, error) {
	var r ParserRun
	err := pool.QueryRow(ctx, `
		SELECT id, started_at, finished_at, source, rows_inserted, rows_swept, sweep, status
		FROM parser_runs
		WHERE status = $1
		ORDER BY started_at DESC
		LIMIT 1
	`, ParserRunSucceeded).Scan(
		&r.ID, &r.StartedAt, &r.FinishedAt, &r.Source,
		&r.RowsInserted, &r.RowsSwept, &r.Sweep, &r.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ParserRun{}, ErrNotFound
		}
		return ParserRun{}, fmt.Errorf("latest succeeded parser run: %w", err)
	}
	return r, nil
}

// ParserRunCounts returns lifetime totals by status. Drives the
// `rutracker_parser_runs_total{status=...}` counter.
type ParserRunCounts struct {
	Succeeded int64
	Failed    int64
	Running   int64
}

// CountParserRuns returns aggregate counts by status. Cheap — the table
// only grows by one row per parse, which is at most a few times a day.
func CountParserRuns(ctx context.Context, pool *pgxpool.Pool) (ParserRunCounts, error) {
	var c ParserRunCounts
	err := pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE status = $1) AS succeeded,
		  COUNT(*) FILTER (WHERE status = $2) AS failed,
		  COUNT(*) FILTER (WHERE status = $3) AS running
		FROM parser_runs
	`, ParserRunSucceeded, ParserRunFailed, ParserRunRunning).
		Scan(&c.Succeeded, &c.Failed, &c.Running)
	if err != nil {
		return ParserRunCounts{}, fmt.Errorf("count parser runs: %w", err)
	}
	return c, nil
}

// RecentParserRuns returns the most recent runs by started_at, newest first.
// Drives the admin panel's "recent loads" list. Empty result (no runs yet) is
// not an error — it returns a nil slice.
func RecentParserRuns(ctx context.Context, pool *pgxpool.Pool, limit int) ([]ParserRun, error) {
	if limit < 1 {
		limit = 3
	}
	rows, err := pool.Query(ctx, `
		SELECT id, started_at, finished_at, source, rows_inserted, rows_swept, sweep, status
		FROM parser_runs
		ORDER BY started_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("recent parser runs: %w", err)
	}
	defer rows.Close()

	var out []ParserRun
	for rows.Next() {
		var r ParserRun
		if err := rows.Scan(
			&r.ID, &r.StartedAt, &r.FinishedAt, &r.Source,
			&r.RowsInserted, &r.RowsSwept, &r.Sweep, &r.Status,
		); err != nil {
			return nil, fmt.Errorf("scan parser run: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recent parser runs rows: %w", err)
	}
	return out, nil
}

// ReconcileRunningRuns marks left-over `running` rows `failed` at startup: a
// single-replica parse can't survive a restart, so a `running` row at boot is
// always an orphan (crash/OOM/deploy) that would otherwise hang status forever.
func ReconcileRunningRuns(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	cmd, err := pool.Exec(ctx, `
		UPDATE parser_runs
		SET status = $1, finished_at = COALESCE(finished_at, NOW())
		WHERE status = $2
	`, ParserRunFailed, ParserRunRunning)
	if err != nil {
		return 0, fmt.Errorf("reconcile running runs: %w", err)
	}
	return cmd.RowsAffected(), nil
}
