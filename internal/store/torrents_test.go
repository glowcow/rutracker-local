package store_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/glowcow/rutracker-local/internal/db"
	"github.com/glowcow/rutracker-local/internal/filelist"
	"github.com/glowcow/rutracker-local/internal/store"
)

// testPool: returns a connected pool with migrations applied and the
// torrents table truncated. Skips the test if DATABASE_URL isn't set, so
// `go test ./...` without a DB still passes (used in lint:go-only contexts).
func testPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	ctx := context.Background()
	if err := db.MigrateUp(ctx, url); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgx pool: %v", err)
	}
	// `favorites`, `torrent_peers` and `torrent_files` all have an FK on
	// torrents(id); listing them keeps TRUNCATE explicit (no CASCADE) so adding
	// another FK-bearing table in the future fails loudly here rather than
	// silently wiping it.
	if _, err := pool.Exec(ctx, `TRUNCATE torrents, favorites, torrent_peers, torrent_files`); err != nil {
		pool.Close()
		t.Fatalf("truncate: %v", err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

// ingest writes batch through the production COPY → merge path (the same
// CopyIngester the parser uses), so every test exercises the SQL that
// actually runs in production. A fresh ingester per call is fine at test
// sizes; the parser holds one per worker for throughput, not correctness.
func ingest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, batch []store.Torrent) int {
	t.Helper()
	ing, err := store.NewCopyIngester(ctx, pool)
	if err != nil {
		t.Fatalf("new copy ingester: %v", err)
	}
	defer ing.Close()
	n, err := ing.Ingest(ctx, batch)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	return n
}

func sampleTorrents() []store.Torrent {
	return []store.Torrent{
		{
			ID: 1, Title: "Amnistia Egotrap FLAC", ForumID: 1868,
			ForumName: "EBM lossless", SizeBytes: 909_382_926,
			RegisteredAt: time.Date(2011, 7, 8, 17, 32, 0, 0, time.UTC),
			Hash:         "AAAA1234567890ABCDEFABCDEFABCDEFABCDEF01",
			Content:      "[size=24]Amnistia[/size]",
		},
		{
			ID: 2, Title: "Pink Floyd Dark Side FLAC", ForumID: 1576,
			ForumName: "Зарубежная музыка (lossless)", SizeBytes: 4_287_419_002,
			RegisteredAt: time.Date(2023, 4, 3, 0, 0, 0, 0, time.UTC),
			Hash:         "BBBB1234567890ABCDEFABCDEFABCDEFABCDEF02",
			Content:      "[b]Pink Floyd[/b]",
		},
		{
			ID: 3, Title: "Дюна 2 BDRemux 2160p", ForumID: 313,
			ForumName: "Зарубежные фильмы (HD Video)", SizeBytes: 68_412_876_543,
			RegisteredAt: time.Date(2024, 4, 25, 0, 0, 0, 0, time.UTC),
			Hash:         "CCCC1234567890ABCDEFABCDEFABCDEFABCDEF03",
			Content:      "[b]Дюна[/b]",
		},
	}
}

func TestGet_RoundTripIncludingContent(t *testing.T) {
	ctx, pool := testPool(t)
	ingest(t, ctx, pool, sampleTorrents())
	got, err := store.Get(ctx, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 1 || got.Title != "Amnistia Egotrap FLAC" {
		t.Errorf("Get returned %+v", got)
	}
	if got.Content == "" {
		t.Errorf("Get should include Content, got empty")
	}
}

func TestGetFiles_RoundTrip(t *testing.T) {
	ctx, pool := testPool(t)
	batch := sampleTorrents()
	batch[0].Files = []filelist.File{
		{Path: "Amnistia/01 Egotrap.flac", Size: 45_112_233},
		{Path: "Amnistia/Scans/front.jpg", Size: 2_048},
	}
	ingest(t, ctx, pool, batch)

	blob, count, err := store.GetFiles(ctx, pool, 1)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if count != 2 {
		t.Errorf("files_count = %d, want 2", count)
	}
	files, err := filelist.Decode(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(files, batch[0].Files) {
		t.Errorf("stored listing = %+v, want %+v", files, batch[0].Files)
	}

	// Get carries the count so the drawer can label a collapsed section.
	got, err := store.Get(ctx, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.FilesCount == nil || *got.FilesCount != 2 {
		t.Errorf("Get().FilesCount = %v, want 2", got.FilesCount)
	}

	// A torrent the dump gave no listing for has no row at all — the API turns
	// that into a 404 and the drawer hides its Files section.
	if _, _, err := store.GetFiles(ctx, pool, 2); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetFiles for a listing-less torrent = %v, want ErrNotFound", err)
	}
	if got2, err := store.Get(ctx, pool, 2); err != nil || got2.FilesCount != nil {
		t.Errorf("Get(2).FilesCount = %v, want nil (err %v)", got2.FilesCount, err)
	}
}

// A sweep must take the listing with it — otherwise vanished torrents leave
// orphaned blobs behind, and the FK would block the delete outright.
func TestSweep_CascadesToFiles(t *testing.T) {
	ctx, pool := testPool(t)
	batch := sampleTorrents()
	batch[0].Files = []filelist.File{{Path: "a.flac", Size: 1}}
	ingest(t, ctx, pool, batch)

	cutoff := time.Now().Add(time.Hour) // everything looks stale
	if _, err := store.Sweep(ctx, pool, cutoff); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM torrent_files`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d listings survived the sweep, want 0", left)
	}
}

func TestGet_NotFound(t *testing.T) {
	ctx, pool := testPool(t)
	_, err := store.Get(ctx, pool, 999_999)
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIngest_InsertThenUpdate(t *testing.T) {
	ctx, pool := testPool(t)

	n := ingest(t, ctx, pool, sampleTorrents())
	if n != 3 {
		t.Fatalf("inserted %d, want 3", n)
	}

	total, err := store.Count(ctx, pool)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total != 3 {
		t.Fatalf("Count = %d, want 3", total)
	}

	// Re-insert same data — the merge UPSERT should keep count at 3.
	ingest(t, ctx, pool, sampleTorrents())
	total, _ = store.Count(ctx, pool)
	if total != 3 {
		t.Fatalf("after re-ingest Count = %d, want 3", total)
	}

	// Mutate one row and verify UPDATE actually changed it.
	rows := sampleTorrents()
	rows[0].Title = "Amnistia Egotrap REMASTER FLAC"
	ingest(t, ctx, pool, rows)
	var got string
	if err := pool.QueryRow(ctx, `SELECT title FROM torrents WHERE id = 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "Amnistia Egotrap REMASTER FLAC" {
		t.Errorf("title not updated: got %q", got)
	}
}

// Identical re-ingest must still refresh last_seen_at — the merge step
// skips no-op rows to keep TOAST stable, so the bump-seen step has to
// refresh the timestamp on its own. If it didn't, the post-parse Sweep
// would delete every row that wasn't materially changed.
func TestIngest_NoOpReinsertStillBumpsLastSeen(t *testing.T) {
	ctx, pool := testPool(t)
	ingest(t, ctx, pool, sampleTorrents())
	if _, err := pool.Exec(ctx,
		`UPDATE torrents SET last_seen_at = NOW() - INTERVAL '1 hour'`); err != nil {
		t.Fatal(err)
	}
	ingest(t, ctx, pool, sampleTorrents())
	var stale int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM torrents WHERE last_seen_at < NOW() - INTERVAL '30 minutes'`,
	).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Errorf("expected all rows to have fresh last_seen_at, %d still stale", stale)
	}
}

func TestSweep_DeletesUntouched(t *testing.T) {
	ctx, pool := testPool(t)

	// Seed the table; everything gets last_seen_at = NOW().
	ingest(t, ctx, pool, sampleTorrents())

	// Capture the cutoff AS the simulated "next parse" would: postgres-side
	// NOW(), then nudge any-but-one row to look like it was re-touched in
	// that future parse.
	cutoff, err := store.ParseStartNow(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE torrents SET last_seen_at = NOW() WHERE id IN (1, 2)`); err != nil {
		t.Fatalf("touch rows: %v", err)
	}

	deleted, err := store.Sweep(ctx, pool, cutoff)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Sweep returned %d, want 1", deleted)
	}

	total, _ := store.Count(ctx, pool)
	if total != 2 {
		t.Errorf("post-sweep Count = %d, want 2", total)
	}
	if _, err := store.Get(ctx, pool, 3); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("id=3 should be gone, got %v", err)
	}
}

func TestSweep_NothingToDelete(t *testing.T) {
	ctx, pool := testPool(t)

	ingest(t, ctx, pool, sampleTorrents())
	// Cutoff captured AFTER seeding — every row has last_seen_at < cutoff,
	// but a real parse would re-touch them all; here we model the "no
	// touches yet" half by simply running sweep against a cutoff BEFORE the
	// seed time. Use a small backdate.
	cutoff := time.Now().Add(-1 * time.Hour)
	deleted, err := store.Sweep(ctx, pool, cutoff)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Sweep deleted %d, want 0", deleted)
	}
}

func TestGetStats(t *testing.T) {
	ctx, pool := testPool(t)
	ingest(t, ctx, pool, sampleTorrents())
	s, err := store.GetStats(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if s.TorrentsTotal != 3 {
		t.Errorf("TorrentsTotal = %d, want 3", s.TorrentsTotal)
	}
	wantSize := int64(909_382_926 + 4_287_419_002 + 68_412_876_543)
	if s.TotalSizeBytes != wantSize {
		t.Errorf("TotalSizeBytes = %d, want %d", s.TotalSizeBytes, wantSize)
	}
	if s.ForumsCount != 3 {
		t.Errorf("ForumsCount = %d, want 3", s.ForumsCount)
	}
}

func TestListForums(t *testing.T) {
	ctx, pool := testPool(t)
	ingest(t, ctx, pool, sampleTorrents())
	forums, err := store.ListForums(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(forums) != 3 {
		t.Fatalf("forums len = %d, want 3", len(forums))
	}
	for _, f := range forums {
		if f.Count != 1 {
			t.Errorf("forum %d count = %d, want 1", f.ID, f.Count)
		}
	}
}

func TestSearch_FTS_MatchesRussian(t *testing.T) {
	ctx, pool := testPool(t)
	ingest(t, ctx, pool, sampleTorrents())
	items, err := store.Search(ctx, pool, store.SearchParams{Query: "FLAC", Sort: "relevance", Dir: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("FLAC items = %d, want 2", len(items))
	}
	total, err := store.CountSearch(ctx, pool, "FLAC", 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("FLAC total = %d, want 2", total)
	}

	items, err = store.Search(ctx, pool, store.SearchParams{Query: "Дюна", Sort: "relevance", Dir: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 3 {
		t.Errorf("Дюна search = %+v, want 1 hit on id=3", items)
	}
	total, err = store.CountSearch(ctx, pool, "Дюна", 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("Дюна total = %d, want 1", total)
	}
}

func TestCountSearch_OffsetPastEnd_StillReportsTotal(t *testing.T) {
	ctx, pool := testPool(t)
	ingest(t, ctx, pool, sampleTorrents())
	// Regression: the old COUNT(*) OVER () form returned total=0 whenever
	// OFFSET landed past the last matching row (no rows → no window).
	items, err := store.Search(ctx, pool, store.SearchParams{Query: "FLAC", Offset: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("offset past end returned %d items, want 0", len(items))
	}
	total, err := store.CountSearch(ctx, pool, "FLAC", 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 even with empty page", total)
	}
}

func TestSearch_Filters_And_Sort(t *testing.T) {
	ctx, pool := testPool(t)
	ingest(t, ctx, pool, sampleTorrents())

	// forum filter
	items, err := store.Search(ctx, pool, store.SearchParams{ForumID: 313, Sort: "date", Dir: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 3 {
		t.Errorf("forum_id=313 = %+v", items)
	}
	total, err := store.CountSearch(ctx, pool, "", 313)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("forum_id=313 total = %d, want 1", total)
	}

	// sort by size desc — biggest first
	items, err = store.Search(ctx, pool, store.SearchParams{Sort: "size", Dir: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].ID != 3 {
		t.Errorf("size desc: expected id=3 first, got %+v", items)
	}

	// sort by date asc — oldest first
	items, err = store.Search(ctx, pool, store.SearchParams{Sort: "date", Dir: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].ID != 1 {
		t.Errorf("date asc: expected id=1 first, got id=%d", items[0].ID)
	}
}

func TestSearch_UnknownSortFallsBackSafely(t *testing.T) {
	ctx, pool := testPool(t)
	ingest(t, ctx, pool, sampleTorrents())
	// Both Sort and Dir contain SQL-injection bait that is not in the
	// whitelist. Search must (a) fall back to "date" / "desc" instead of
	// concatenating user input into ORDER BY, and (b) leave the table intact.
	items, err := store.Search(ctx, pool, store.SearchParams{
		Sort: "banana; DROP TABLE torrents;--",
		Dir:  "banana",
	})
	if err != nil {
		t.Fatalf("unknown sort errored: %v", err)
	}
	if items[0].ID != 3 { // newest by registered_at — date desc
		t.Errorf("fallback expected newest (id=3) first, got id=%d", items[0].ID)
	}
	count, err := store.Count(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("table count = %d, want 3 — injection got through?", count)
	}
}
