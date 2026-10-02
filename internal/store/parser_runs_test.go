package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/glowcow/rutracker-local/internal/store"
)

// truncateParserRuns wipes parser_runs between tests so each starts with a
// known-empty table. Mirrors what testPool does for the torrents table.
func truncateParserRuns(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE parser_runs RESTART IDENTITY`); err != nil {
		t.Fatalf("truncate parser_runs: %v", err)
	}
}

func TestParserRun_StartFinishLifecycle(t *testing.T) {
	ctx, pool := testPool(t)
	truncateParserRuns(t, ctx, pool)

	id, err := store.StartParserRun(ctx, pool, "/dumps/test.xml.xz", true)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if id <= 0 {
		t.Fatalf("Start returned id=%d", id)
	}

	got, err := store.LatestParserRun(ctx, pool)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got.ID != id {
		t.Errorf("Latest id = %d, want %d", got.ID, id)
	}
	if got.Status != store.ParserRunRunning {
		t.Errorf("Latest status = %q, want running", got.Status)
	}
	if !got.Sweep {
		t.Errorf("Latest sweep = false, want true")
	}
	if got.FinishedAt != nil {
		t.Errorf("Latest finished_at non-nil while running: %v", got.FinishedAt)
	}

	if err := store.FinishParserRun(ctx, pool, id, 1234, 56); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got, err = store.LatestParserRun(ctx, pool)
	if err != nil {
		t.Fatalf("Latest after finish: %v", err)
	}
	if got.Status != store.ParserRunSucceeded {
		t.Errorf("Status = %q, want succeeded", got.Status)
	}
	if got.FinishedAt == nil {
		t.Fatalf("finished_at nil after Finish")
	}
	if got.RowsInserted != 1234 {
		t.Errorf("RowsInserted = %d, want 1234", got.RowsInserted)
	}
	if got.RowsSwept != 56 {
		t.Errorf("RowsSwept = %d, want 56", got.RowsSwept)
	}
}

func TestParserRun_Fail(t *testing.T) {
	ctx, pool := testPool(t)
	truncateParserRuns(t, ctx, pool)

	id, err := store.StartParserRun(ctx, pool, "/dumps/test.xml.xz", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FailParserRun(ctx, pool, id); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	got, err := store.LatestParserRun(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.ParserRunFailed {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.FinishedAt == nil {
		t.Errorf("finished_at nil after Fail")
	}
}

func TestParserRun_LatestEmptyReturnsNotFound(t *testing.T) {
	ctx, pool := testPool(t)
	truncateParserRuns(t, ctx, pool)

	_, err := store.LatestParserRun(ctx, pool)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Latest on empty table: err = %v, want ErrNotFound", err)
	}
}

func TestParserRun_Counts(t *testing.T) {
	ctx, pool := testPool(t)
	truncateParserRuns(t, ctx, pool)

	// Seed: 2 succeeded, 1 failed, 1 still running.
	for i := 0; i < 2; i++ {
		id, err := store.StartParserRun(ctx, pool, "/x", false)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.FinishParserRun(ctx, pool, id, int64(100*(i+1)), 0); err != nil {
			t.Fatal(err)
		}
	}
	id, err := store.StartParserRun(ctx, pool, "/x", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FailParserRun(ctx, pool, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartParserRun(ctx, pool, "/x", false); err != nil {
		t.Fatal(err)
	}

	counts, err := store.CountParserRuns(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", counts.Succeeded)
	}
	if counts.Failed != 1 {
		t.Errorf("Failed = %d, want 1", counts.Failed)
	}
	if counts.Running != 1 {
		t.Errorf("Running = %d, want 1", counts.Running)
	}
}
