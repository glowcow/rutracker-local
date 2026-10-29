package ingest

import (
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/glowcow/rutracker-local/internal/db"
)

// scoped gives the test a database of its own: the store tests truncate the
// shared one at the same time.
func scoped(t *testing.T) string {
	t.Helper()
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(u.Path, "/") + "_ingest"
	admin, err := pgxpool.New(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	for _, q := range []string{`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`, `CREATE DATABASE "` + name + `"`} {
		if _, err := admin.Exec(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	u.Path = "/" + name
	if err := db.MigrateUp(t.Context(), u.String()); err != nil {
		t.Fatal(err)
	}
	return u.String()
}

func TestRun(t *testing.T) {
	dsn := scoped(t)
	var (
		mu     sync.Mutex
		status []string
	)
	emit := func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		if ev.Kind == "status" {
			status = append(status, ev.Status)
		}
	}

	res, err := Run(t.Context(), dsn, Options{Source: "../parser/testdata/tiny.xml", Sweep: true}, emit)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var rows int64
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM torrents`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if res.RunID == 0 || res.Rows == 0 || rows != res.Rows || res.Swept != 0 {
		t.Errorf("first run %+v, %d rows in the table", res, rows)
	}
	if strings.Join(status, ",") != "running,succeeded" {
		t.Errorf("status events %v", status)
	}

	// The same dump again sweeps nothing and leaves the set as it was.
	again, err := Run(t.Context(), dsn, Options{Source: "../parser/testdata/tiny.xml", Sweep: true}, nil)
	if err != nil || again.Rows != res.Rows || again.Swept != 0 || again.RunID <= res.RunID {
		t.Errorf("second run %+v %v", again, err)
	}
}

func TestRunRefusesBadInput(t *testing.T) {
	if _, err := Run(t.Context(), "postgres://unused", Options{}, nil); err == nil {
		t.Error("a run without a source started")
	}
	dsn := scoped(t)
	if _, err := Run(t.Context(), dsn, Options{Source: "../parser/testdata/none.xml"}, nil); err == nil {
		t.Error("a missing dump succeeded")
	}
}
