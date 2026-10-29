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

// torrentCols are the input columns, no generated search_vec; the stage DDL
// and the INSERT…SELECT share them, so the two cannot drift.
var torrentCols = []string{
	"id", "title", "forum_id", "forum_name",
	"size_bytes", "registered_at", "hash", "content",
}

// torrentFileCols mirrors torrentCols for the companion listing table.
var torrentFileCols = []string{"torrent_id", "files_count", "files"}

// Torrent is one dump entry as the parser, the store and the API see it;
// fields follow the SQL columns.
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
	// Cached peer counts, nil until first checked; PeersCheckedAt lets the UI
	// grey out values older than the TTL.
	Seeders        *int       `json:"seeders,omitempty"`
	Leechers       *int       `json:"leechers,omitempty"`
	PeersCheckedAt *time.Time `json:"peers_checked_at,omitempty"`
	// Files is the dump's file listing, flattened to slash-joined paths. Never
	// serialised with the torrent — the drawer pulls it from its own endpoint.
	Files []filelist.File `json:"-"`
	// FilesCount labels the drawer's folded file list without the tree; nil on
	// list reads and for a dump without listings.
	FilesCount *int `json:"files_count,omitempty"`
}

// CopyIngester holds one connection, whose session keeps its settings and the
// TEMP stage table across Ingest calls; one per goroutine.
type CopyIngester struct {
	conn            *pgxpool.Conn
	stageTable      pgx.Identifier
	filesStageTable pgx.Identifier
}

// NewCopyIngester takes a connection with synchronous_commit off (a reload is
// idempotent) and its own stage table; Close releases it.
func NewCopyIngester(ctx context.Context, pool *pgxpool.Pool) (*CopyIngester, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire conn: %w", err)
	}
	if _, err := conn.Exec(ctx, `SET synchronous_commit = off`); err != nil {
		conn.Release()
		return nil, fmt.Errorf("set sync_commit: %w", err)
	}
	// A TEMP table is per session, so every ingester can use the same name.
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

// copyIngestMerge writes a row only when it differs from the dump, so a
// re-parse touches almost nothing; last_seen_at is the database's NOW().
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

// copyIngestBumpSeen touches last_seen_at of rows the merge skipped, or the
// sweep would take them for gone.
const copyIngestBumpSeen = `
UPDATE torrents SET last_seen_at = NOW()
WHERE id IN (SELECT id FROM torrents_stage)
`

// copyIngestMergeFiles leaves identical listings alone, as the merge does;
// the cascade from torrents expires them.
const copyIngestMergeFiles = `
INSERT INTO torrent_files (torrent_id, files_count, files)
SELECT torrent_id, files_count, files
FROM torrent_files_stage
ON CONFLICT (torrent_id) DO UPDATE SET
	files_count = EXCLUDED.files_count,
	files       = EXCLUDED.files
WHERE torrent_files.files IS DISTINCT FROM EXCLUDED.files
`

// Ingest merges the batch through COPY and INSERT…ON CONFLICT and returns the
// rows merged; the stage table is emptied after.
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

// ingestFiles stores each listing compressed; a torrent without one gets no
// row, which the API reads as "no file data".
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

// GetFiles returns the compressed listing and the true file count;
// ErrNotFound when the dump had none.
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

// ParseStartNow is the database's now, the sweep cutoff: every NOW() of the
// parse is later, whatever the clocks say.
func ParseStartNow(ctx context.Context, pool *pgxpool.Pool) (time.Time, error) {
	var t time.Time
	if err := pool.QueryRow(ctx, `SELECT NOW()`).Scan(&t); err != nil {
		return time.Time{}, fmt.Errorf("parse start now: %w", err)
	}
	return t, nil
}

// Sweep deletes the torrents a full parse did not see since before, its
// ParseStartNow, and returns how many.
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

// TableBloat is a relation's live and dead tuples; dead/(live+dead) above 20%
// that does not fall back means autovacuum is behind.
type TableBloat struct {
	LiveTuples int64
	DeadTuples int64
}

// GetTableBloat reads the estimates of pg_stat_user_tables for name.
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

// DatabaseSizeBytes is pg_database_size: heap, indexes, TOAST and dead space.
func DatabaseSizeBytes(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	err := pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("database size: %w", err)
	}
	return n, nil
}

// GetStats scans the whole table, fine for a few tens of millions of rows.
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

// SearchParams is a search request; a zero field is no filter.
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

// maxOffset bounds paging: OFFSET is linear in postgres, and 2000 pages is
// more than anyone reads.
const maxOffset = 100_000

// maxQueryLen bounds the raw search string before it hits plainto_tsquery.
const maxQueryLen = 256

// searchFilter builds the WHERE of Search and CountSearch from the set filters
// only: a catch-all predicate defeats the index under prepared statements.
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

// Search returns one page; COUNT(*) OVER () here would materialise every
// match before LIMIT, so the total is CountSearch's.
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
	// The peer cache joins torrents, not q, so the precedence stays
	// (torrents LEFT JOIN torrent_peers), q.
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

// CountSearch counts Search's matches; apart, the handler caches it per dump
// and an offset past the end still gets the real total.
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
