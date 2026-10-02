// Package store owns the canonical Torrent type and persists it to Postgres.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/glowcow/rutracker-local/internal/filelist"
)

// torrentCols enumerates the input columns (no GENERATED search_vec). Shared
// between the COPY stage table DDL and the INSERT...SELECT column list so the
// two can never drift.
var torrentCols = []string{
	"id", "title", "forum_id", "forum_name",
	"size_bytes", "registered_at", "hash", "content",
}

// torrentFileCols mirrors torrentCols for the companion listing table.
var torrentFileCols = []string{"torrent_id", "files_count", "files"}

// Torrent is the shared in-memory representation used by parser, store, and
// (eventually) the HTTP handlers. Field order matches the SQL columns we
// INSERT below so adding/removing a field is a one-line change.
type Torrent struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	ForumID      int32     `json:"forum_id"`
	ForumName    string    `json:"forum_name"`
	SizeBytes    int64     `json:"size_bytes"`
	RegisteredAt time.Time `json:"registered_at"`
	Hash         string    `json:"hash"` // 40-char hex SHA1, uppercase
	// Content is omitted from list responses (too heavy); single-torrent
	// endpoints will explicitly include it (rendered HTML).
	Content string `json:"-"`
	// Peers — cached seeders/leechers joined from torrent_peers on list reads.
	// Nil when a torrent has never been checked; omitted from JSON so unchecked
	// rows stay lean. PeersCheckedAt lets the UI grey out values past the TTL.
	Seeders        *int       `json:"seeders,omitempty"`
	Leechers       *int       `json:"leechers,omitempty"`
	PeersCheckedAt *time.Time `json:"peers_checked_at,omitempty"`
	// Files is the dump's file listing, flattened to slash-joined paths. Never
	// serialised with the torrent — the drawer pulls it from its own endpoint.
	Files []filelist.File `json:"-"`
	// FilesCount comes from the companion table on single-torrent reads, so the
	// drawer can label its (collapsed) Files section without fetching the tree.
	// Nil on list reads and when the dump carried no listing.
	FilesCount *int `json:"files_count,omitempty"`
}

// CopyIngester is a per-worker ingestion handle: it holds one pooled connection
// for its lifetime so session state (synchronous_commit=off, TEMP stage table)
// outlives each Ingest call. NOT safe for concurrent use — one per goroutine.
type CopyIngester struct {
	conn            *pgxpool.Conn
	stageTable      pgx.Identifier
	filesStageTable pgx.Identifier
}

// NewCopyIngester acquires a pooled conn, sets synchronous_commit=off, and
// creates a session-local TEMP stage table (no GENERATED tsvector — that
// materialises only on the final INSERT). sync_commit=off drops the per-COMMIT
// fsync — safe because a bulk reload is idempotent (re-parse on crash), and
// it's the biggest throughput win here. Caller must Close to release the conn.
func NewCopyIngester(ctx context.Context, pool *pgxpool.Pool) (*CopyIngester, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire conn: %w", err)
	}
	if _, err := conn.Exec(ctx, `SET synchronous_commit = off`); err != nil {
		conn.Release()
		return nil, fmt.Errorf("set sync_commit: %w", err)
	}
	// TEMP TABLE name is per-session — concurrent ingesters can reuse the
	// same identifier without collision. ON COMMIT PRESERVE ROWS isn't
	// needed (we run outside an explicit txn) but is harmless and documents
	// intent.
	const stageDDL = `
	CREATE TEMP TABLE IF NOT EXISTS torrents_stage (
		id            BIGINT,
		title         TEXT,
		forum_id      INT,
		forum_name    TEXT,
		size_bytes    BIGINT,
		registered_at TIMESTAMPTZ,
		hash          CHAR(40),
		content       TEXT
	)`
	if _, err := conn.Exec(ctx, stageDDL); err != nil {
		conn.Release()
		return nil, fmt.Errorf("create stage table: %w", err)
	}
	const filesStageDDL = `
	CREATE TEMP TABLE IF NOT EXISTS torrent_files_stage (
		torrent_id  BIGINT,
		files_count INT,
		files       BYTEA
	)`
	if _, err := conn.Exec(ctx, filesStageDDL); err != nil {
		conn.Release()
		return nil, fmt.Errorf("create files stage table: %w", err)
	}
	return &CopyIngester{
		conn:            conn,
		stageTable:      pgx.Identifier{"torrents_stage"},
		filesStageTable: pgx.Identifier{"torrent_files_stage"},
	}, nil
}

// Close releases the underlying connection back to the pool. The TEMP table
// is dropped automatically when the session ends.
func (c *CopyIngester) Close() {
	if c.conn != nil {
		c.conn.Release()
		c.conn = nil
	}
}

// Step 1 of the two-step ingest: write a row only when it differs from the dump
// — identical re-parses (~95%+ in steady state) are no-ops at the heap/TOAST
// level. last_seen_at = SQL NOW() (not COPY'd) so every merged row gets a fresh
// timestamp.
const copyIngestMerge = `
INSERT INTO torrents (id, title, forum_id, forum_name, size_bytes, registered_at, hash, content, last_seen_at)
SELECT id, title, forum_id, forum_name, size_bytes, registered_at, hash, content, NOW()
FROM torrents_stage
ON CONFLICT (id) DO UPDATE SET
	title         = EXCLUDED.title,
	forum_id      = EXCLUDED.forum_id,
	forum_name    = EXCLUDED.forum_name,
	size_bytes    = EXCLUDED.size_bytes,
	registered_at = EXCLUDED.registered_at,
	hash          = EXCLUDED.hash,
	content       = EXCLUDED.content,
	last_seen_at  = EXCLUDED.last_seen_at
WHERE
	torrents.title            IS DISTINCT FROM EXCLUDED.title
	OR torrents.forum_id      IS DISTINCT FROM EXCLUDED.forum_id
	OR torrents.forum_name    IS DISTINCT FROM EXCLUDED.forum_name
	OR torrents.size_bytes    IS DISTINCT FROM EXCLUDED.size_bytes
	OR torrents.registered_at IS DISTINCT FROM EXCLUDED.registered_at
	OR torrents.hash          IS DISTINCT FROM EXCLUDED.hash
	OR torrents.content       IS DISTINCT FROM EXCLUDED.content
`

// Step 2 — heap-only UPDATE that bumps last_seen_at on every input id.
// Needed because step 1's WHERE clause skips no-op rows entirely, which
// would otherwise leave their last_seen_at stale and let Sweep mistake
// them for vanished entries.
const copyIngestBumpSeen = `
UPDATE torrents SET last_seen_at = NOW()
WHERE id IN (SELECT id FROM torrents_stage)
`

// File listings are merged the same way as step 1: identical blobs are left
// alone, so a re-parse of the same dump doesn't rewrite ~2.8M TOAST entries.
// No last_seen_at here — the FK cascade from torrents is what expires rows.
const copyIngestMergeFiles = `
INSERT INTO torrent_files (torrent_id, files_count, files)
SELECT torrent_id, files_count, files
FROM torrent_files_stage
ON CONFLICT (torrent_id) DO UPDATE SET
	files_count = EXCLUDED.files_count,
	files       = EXCLUDED.files
WHERE torrent_files.files IS DISTINCT FROM EXCLUDED.files
`

// Ingest writes the batch into Postgres via COPY → INSERT...ON CONFLICT.
// The stage table is truncated at the end so the next batch starts empty.
// Returns the number of rows merged.
func (c *CopyIngester) Ingest(ctx context.Context, batch []Torrent) (int, error) {
	if len(batch) == 0 {
		return 0, nil
	}
	n, err := c.conn.CopyFrom(ctx,
		c.stageTable,
		torrentCols,
		pgx.CopyFromSlice(len(batch), func(i int) ([]any, error) {
			t := batch[i]
			return []any{
				t.ID, t.Title, t.ForumID, t.ForumName,
				t.SizeBytes, t.RegisteredAt, t.Hash, t.Content,
			}, nil
		}),
	)
	if err != nil {
		return 0, fmt.Errorf("copy stage: %w", err)
	}
	if _, err := c.conn.Exec(ctx, copyIngestMerge); err != nil {
		return 0, fmt.Errorf("merge stage: %w", err)
	}
	if _, err := c.conn.Exec(ctx, copyIngestBumpSeen); err != nil {
		return 0, fmt.Errorf("bump last_seen_at: %w", err)
	}
	// After the torrents merge — the listing's FK needs those rows to exist.
	if err := c.ingestFiles(ctx, batch); err != nil {
		return 0, err
	}
	// TRUNCATE is faster than DELETE and resets the table for the next batch.
	if _, err := c.conn.Exec(ctx, `TRUNCATE torrents_stage`); err != nil {
		return 0, fmt.Errorf("truncate stage: %w", err)
	}
	return int(n), nil
}

// ingestFiles compresses each torrent's listing and merges the batch into
// torrent_files. Torrents without a listing (older dumps carry none) are
// skipped rather than stored empty, so "no row" stays the single signal the
// API reads as "this dump had no file data".
func (c *CopyIngester) ingestFiles(ctx context.Context, batch []Torrent) error {
	type row struct {
		id    int64
		count int
		blob  []byte
	}
	rows := make([]row, 0, len(batch))
	for _, t := range batch {
		if len(t.Files) == 0 {
			continue
		}
		blob, err := filelist.Encode(t.Files)
		if err != nil {
			return fmt.Errorf("encode file list for %d: %w", t.ID, err)
		}
		rows = append(rows, row{id: t.ID, count: len(t.Files), blob: blob})
	}
	if len(rows) == 0 {
		return nil
	}
	if _, err := c.conn.CopyFrom(ctx,
		c.filesStageTable,
		torrentFileCols,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			return []any{rows[i].id, rows[i].count, rows[i].blob}, nil
		}),
	); err != nil {
		return fmt.Errorf("copy files stage: %w", err)
	}
	if _, err := c.conn.Exec(ctx, copyIngestMergeFiles); err != nil {
		return fmt.Errorf("merge files stage: %w", err)
	}
	if _, err := c.conn.Exec(ctx, `TRUNCATE torrent_files_stage`); err != nil {
		return fmt.Errorf("truncate files stage: %w", err)
	}
	return nil
}

// GetFiles returns a torrent's stored listing: the compressed blob and the
// true file count. ErrNotFound means the dump carried no listing for it (or
// the torrent itself is gone) — the drawer then hides the section.
func GetFiles(ctx context.Context, pool *pgxpool.Pool, id int64) ([]byte, int, error) {
	var (
		blob  []byte
		count int
	)
	err := pool.QueryRow(ctx, `
		SELECT files, files_count FROM torrent_files WHERE torrent_id = $1
	`, id).Scan(&blob, &count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("get files %d: %w", id, err)
	}
	return blob, count, nil
}

// ParseStartNow returns the postgres-side "now", used as the Sweep cutoff.
// Using the db clock (not Go's) sidesteps skew: every NOW() the parse touches
// is ≥ this, so Sweep won't clobber rows it just updated.
func ParseStartNow(ctx context.Context, pool *pgxpool.Pool) (time.Time, error) {
	var t time.Time
	if err := pool.QueryRow(ctx, `SELECT NOW()`).Scan(&t); err != nil {
		return time.Time{}, fmt.Errorf("parse start now: %w", err)
	}
	return t, nil
}

// Sweep deletes torrents whose last_seen_at is older than `before` — i.e.,
// rows that no longer exist in the current dump. Returns the number of rows
// deleted. Intended to be called once at the end of a full parse, with
// `before` set to the value of ParseStartNow taken before the parse began.
func Sweep(ctx context.Context, pool *pgxpool.Pool, before time.Time) (int64, error) {
	cmd, err := pool.Exec(ctx, `DELETE FROM torrents WHERE last_seen_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("sweep: %w", err)
	}
	return cmd.RowsAffected(), nil
}

// ErrNotFound is returned by Get when the row doesn't exist. Callers map
// it to a 404; any other error is a 500.
var ErrNotFound = fmt.Errorf("torrent not found")

// Get fetches a single torrent INCLUDING the BBCode content field, which is
// otherwise omitted from list responses.
func Get(ctx context.Context, pool *pgxpool.Pool, id int64) (Torrent, error) {
	var t Torrent
	err := pool.QueryRow(ctx, `
		SELECT t.id, t.title, t.forum_id, t.forum_name, t.size_bytes,
		       t.registered_at, t.hash, t.content, f.files_count
		FROM torrents t
		LEFT JOIN torrent_files f ON f.torrent_id = t.id
		WHERE t.id = $1
	`, id).Scan(
		&t.ID, &t.Title, &t.ForumID, &t.ForumName,
		&t.SizeBytes, &t.RegisteredAt, &t.Hash, &t.Content, &t.FilesCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Torrent{}, ErrNotFound
		}
		return Torrent{}, fmt.Errorf("get torrent %d: %w", id, err)
	}
	return t, nil
}

// Count returns the number of rows in the torrents table.
func Count(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM torrents`).Scan(&n)
	return n, err
}

// Stats — aggregate numbers for the dashboard / sidebar panel.
type Stats struct {
	TorrentsTotal  int64 `json:"torrents_total"`
	TotalSizeBytes int64 `json:"total_size_bytes"`
	ForumsCount    int64 `json:"forums_count"`
}

// TableBloat is what pg_stat_user_tables tracks per relation. The ratio
// `Dead / (Live + Dead)` is the standard signal for autovacuum keeping up
// — once it pushes past ~20% with no downward trend, consider VACUUM FULL.
type TableBloat struct {
	LiveTuples int64
	DeadTuples int64
}

// GetTableBloat reads n_live_tup / n_dead_tup from pg_stat_user_tables for
// the given relation. These are statistics-collector estimates, refreshed
// by analyze/autoanalyze, not exact counts — good enough as a bloat trend.
func GetTableBloat(ctx context.Context, pool *pgxpool.Pool, name string) (TableBloat, error) {
	var b TableBloat
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(n_live_tup, 0), COALESCE(n_dead_tup, 0)
		FROM pg_stat_user_tables
		WHERE relname = $1
	`, name).Scan(&b.LiveTuples, &b.DeadTuples)
	if err != nil {
		return TableBloat{}, fmt.Errorf("table bloat %s: %w", name, err)
	}
	return b, nil
}

// DatabaseSizeBytes wraps pg_database_size — total on-disk footprint for
// the current database (heap + indexes + TOAST + dead space). Useful for
// noticing a slow drift up between parses even when row counts are flat.
func DatabaseSizeBytes(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	err := pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("database size: %w", err)
	}
	return n, nil
}

// GetStats does one full scan. Acceptable up to a few tens of millions on
// homelab hardware; if it gets slow we'll cache in a meta row maintained by
// the parser.
func GetStats(ctx context.Context, pool *pgxpool.Pool) (Stats, error) {
	var s Stats
	err := pool.QueryRow(ctx, `
		SELECT
		  COUNT(*)                                  AS torrents_total,
		  COALESCE(SUM(size_bytes), 0)              AS total_size_bytes,
		  COUNT(DISTINCT forum_id)                  AS forums_count
		FROM torrents
	`).Scan(&s.TorrentsTotal, &s.TotalSizeBytes, &s.ForumsCount)
	return s, err
}

// Forum is one row of the forums summary.
type Forum struct {
	ID    int32  `json:"id"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// ListForums groups by forum_id and returns id/name/count. We pick the first
// name we see per id (MIN) — names are stable per id within a single dump.
func ListForums(ctx context.Context, pool *pgxpool.Pool) ([]Forum, error) {
	rows, err := pool.Query(ctx, `
		SELECT forum_id, MIN(forum_name) AS forum_name, COUNT(*) AS torrent_count
		FROM torrents
		GROUP BY forum_id
		ORDER BY torrent_count DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("forums query: %w", err)
	}
	defer rows.Close()

	var out []Forum
	for rows.Next() {
		var f Forum
		if err := rows.Scan(&f.ID, &f.Name, &f.Count); err != nil {
			return nil, fmt.Errorf("forums scan: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SearchParams is the read-side request shape. All filter fields use the
// zero value as "no filter" (forum_id=0 means "any forum", etc). Frontend
// passes only the fields the user has set.
type SearchParams struct {
	Query   string
	ForumID int32
	Sort    string // "relevance" | "date" | "size"
	Dir     string // "asc" | "desc"
	Offset  int
	Limit   int
}

// SearchResult is the /api/search response shape; the handler composes it
// from Search (page items) and CountSearch (total, cached per dump version).
type SearchResult struct {
	Items []Torrent `json:"items"`
	Total int64     `json:"total"`
}

// validSort/validDir centralise the whitelists. Unknown values fall through
// to safe defaults inside Search.
var (
	validSort = map[string]bool{"relevance": true, "date": true, "size": true}
	validDir  = map[string]bool{"asc": true, "desc": true}
)

// maxOffset bounds how deep pagination can reach: OFFSET is O(n) in postgres,
// so an arbitrary value lets one request scan/sort the whole table. 100k rows
// (2000 pages) is far beyond anything a human pages through.
const maxOffset = 100_000

// maxQueryLen bounds the raw search string before it hits plainto_tsquery.
const maxQueryLen = 256

// searchFilter builds the shared WHERE + args (Search/CountSearch). Conditions
// are appended only when set — the old catch-all ($1=” OR search_vec @@ q.q)
// couldn't use an index on browse and risked a generic-plan seq-scan under pgx
// prepared statements. $1 always feeds the tsquery CTE.
func searchFilter(query string, forumID int32) (where string, args []any) {
	args = []any{query}
	var conds []string
	if query != "" {
		conds = append(conds, "search_vec @@ q.q")
	}
	if forumID != 0 {
		args = append(args, forumID)
		conds = append(conds, fmt.Sprintf("forum_id = $%d", len(args)))
	}
	if len(conds) == 0 {
		return "TRUE", args
	}
	return strings.Join(conds, " AND "), args
}

// clampQuery applies the shared input bounds for Search/CountSearch. Must be
// identical in both paths, or the cached total won't match the page items.
func clampQuery(query string) string {
	if len(query) > maxQueryLen {
		return query[:maxQueryLen]
	}
	return query
}

// Search returns up to Limit page rows. The total lives in CountSearch on
// purpose: COUNT(*) OVER () here would materialise every match before LIMIT
// (the empty-query browse scanned all ~2.8M rows per page instead of 50).
func Search(ctx context.Context, pool *pgxpool.Pool, p SearchParams) ([]Torrent, error) {
	if !validSort[p.Sort] {
		p.Sort = "date"
	}
	if !validDir[p.Dir] {
		p.Dir = "desc"
	}
	if p.Limit <= 0 || p.Limit > 200 {
		p.Limit = 50
	}
	if p.Offset < 0 || p.Offset > maxOffset {
		p.Offset = 0
	}
	p.Query = clampQuery(p.Query)

	// ORDER BY is built from the sort whitelist — never from raw user input,
	// so this is not SQL injection territory.
	dir := strings.ToUpper(p.Dir)
	var orderBy string
	switch p.Sort {
	case "relevance":
		if p.Query == "" {
			orderBy = "registered_at DESC"
		} else {
			orderBy = "ts_rank(search_vec, q.q) " + dir
		}
	case "date":
		orderBy = "registered_at " + dir
	case "size":
		orderBy = "size_bytes " + dir
	}

	where, args := searchFilter(p.Query, p.ForumID)
	args = append(args, p.Limit, p.Offset)
	// LEFT JOIN the live peer cache so list rows carry seeders/leechers when
	// known. The join goes on torrents (not q) — the comma-cross with q stays
	// to the right so precedence keeps `(torrents LEFT JOIN torrent_peers), q`.
	sql := fmt.Sprintf(`
		WITH q AS (SELECT plainto_tsquery('russian', $1) AS q)
		SELECT id, title, forum_id, forum_name, size_bytes, registered_at, hash,
		       tp.seeders, tp.leechers, tp.checked_at
		FROM torrents LEFT JOIN torrent_peers tp ON tp.torrent_id = torrents.id, q
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, where, orderBy, len(args)-1, len(args))

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	items := make([]Torrent, 0, p.Limit)
	for rows.Next() {
		var t Torrent
		if err := rows.Scan(
			&t.ID, &t.Title, &t.ForumID, &t.ForumName,
			&t.SizeBytes, &t.RegisteredAt, &t.Hash,
			&t.Seeders, &t.Leechers, &t.PeersCheckedAt,
		); err != nil {
			return nil, fmt.Errorf("search scan: %w", err)
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

// CountSearch returns the total number of rows matching the same filters
// Search applies. Split from Search so the handler can cache it per dump
// version — and so an offset past the last row still reports the real total
// (the old COUNT(*) OVER () form returned 0 there: no rows, no window).
func CountSearch(ctx context.Context, pool *pgxpool.Pool, query string, forumID int32) (int64, error) {
	where, args := searchFilter(clampQuery(query), forumID)
	sql := fmt.Sprintf(`
		WITH q AS (SELECT plainto_tsquery('russian', $1) AS q)
		SELECT COUNT(*)
		FROM torrents, q
		WHERE %s
	`, where)

	var total int64
	if err := pool.QueryRow(ctx, sql, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("search count: %w", err)
	}
	return total, nil
}
