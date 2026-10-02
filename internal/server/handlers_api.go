package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/glowcow/rutracker-local/internal/bbcode"
	"github.com/glowcow/rutracker-local/internal/filelist"
	"github.com/glowcow/rutracker-local/internal/store"
)

// API handlers — all live behind the /api/ prefix in server.Run.

// versionedCache memoises an expensive read per version = latest succeeded
// parser_run id (torrents is immutable between sweeps, so recompute only on a
// new sweep). In-process, dies on restart. Version 0 ("no succeeded run") is
// never cached — it conflates fresh-deploy / mid-parse / DB-error states.
type versionedCache[T any] struct {
	mu        sync.Mutex
	version   int64
	populated bool
	value     T
}

func (c *versionedCache[T]) get(version int64) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if version != 0 && c.populated && c.version == version {
		return c.value, true
	}
	var zero T
	return zero, false
}

func (c *versionedCache[T]) set(version int64, v T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Refuse the sentinel and stale writers: a request that read version N
	// before a sweep finished must not overwrite the N+1 snapshot a faster
	// request already stored.
	if version == 0 || (c.populated && version < c.version) {
		return
	}
	c.version = version
	c.value = v
	c.populated = true
}

// latestParserRun returns the latest *succeeded* parser_run: its id (the cache
// version) and finish time (the dump's freshness stamp, nil while unfinished).
// Zero id = none yet (fresh deploy / all failed); non-NotFound errors are
// logged but return 0 (which versionedCache treats as uncacheable → recompute).
func latestParserRun(ctx context.Context, pool *pgxpool.Pool) (int64, *time.Time) {
	pr, err := store.LatestSucceededParserRun(ctx, pool)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("parser_run version lookup", "err", err)
		}
		return 0, nil
	}
	return pr.ID, pr.FinishedAt
}

// latestParserVersion is the id-only form used by the caches/ETags.
func latestParserVersion(ctx context.Context, pool *pgxpool.Pool) int64 {
	id, _ := latestParserRun(ctx, pool)
	return id
}

// applyVersionETag stamps a dump-version ETag and answers If-None-Match with a
// 304 (the parser_run id is a perfect validator — /api/forums changes only when
// it does). no-cache = revalidate each load, but a 304 beats the 70 KB body.
func applyVersionETag(w http.ResponseWriter, r *http.Request, version int64) bool {
	if version == 0 {
		return false
	}
	etag := `"v` + strconv.FormatInt(version, 10) + `"`
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

func statsHandler(pool *pgxpool.Pool, peersEnabled bool) http.HandlerFunc {
	// statsResp = the heavy dump-derived stats (cached by parser version) plus
	// peers_cached, which grows continuously as browsing populates the peer
	// cache. peers_cached is recomputed per request (cheap COUNT on a small
	// table), so no version-ETag here — it would 304 away the changing number.
	// peers_enabled lets the list hide its peer badge when the feature is off.
	// dump_updated_at is when the last successful ingest finished — the UI's
	// "@ dd-mm-yyyy" freshness stamp; omitted until a first sweep completes.
	type statsResp struct {
		store.Stats
		PeersCached   int64      `json:"peers_cached"`
		PeersEnabled  bool       `json:"peers_enabled"`
		DumpUpdatedAt *time.Time `json:"dump_updated_at,omitempty"`
	}
	cache := &versionedCache[store.Stats]{}
	return func(w http.ResponseWriter, r *http.Request) {
		key, dumpUpdatedAt := latestParserRun(r.Context(), pool)
		s, ok := cache.get(key)
		if !ok {
			var err error
			s, err = store.GetStats(r.Context(), pool)
			if err != nil {
				slog.Error("stats", "err", err)
				writeJSON(w, http.StatusInternalServerError, errResp("stats failed"))
				return
			}
			cache.set(key, s)
		}
		peersCached, err := store.CountCachedPeers(r.Context(), pool)
		if err != nil {
			slog.Warn("peers cached count", "err", err)
		}
		writeJSON(w, http.StatusOK, statsResp{
			Stats:         s,
			PeersCached:   peersCached,
			PeersEnabled:  peersEnabled,
			DumpUpdatedAt: dumpUpdatedAt,
		})
	}
}

func forumsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	cache := &versionedCache[[]store.Forum]{}
	return func(w http.ResponseWriter, r *http.Request) {
		key := latestParserVersion(r.Context(), pool)
		if applyVersionETag(w, r, key) {
			return
		}
		if cached, ok := cache.get(key); ok {
			writeJSON(w, http.StatusOK, map[string]any{"items": cached})
			return
		}
		forums, err := store.ListForums(r.Context(), pool)
		if err != nil {
			slog.Error("forums", "err", err)
			writeJSON(w, http.StatusInternalServerError, errResp("forums failed"))
			return
		}
		// `[]Forum(nil)` would marshal as `null`; coerce to empty slice.
		if forums == nil {
			forums = []store.Forum{}
		}
		cache.set(key, forums)
		writeJSON(w, http.StatusOK, map[string]any{"items": forums})
	}
}

// searchTotalsCache memoises CountSearch per (q, forum_id) for one dump version.
// The count is the expensive half of a search (a broad query counts millions),
// changing only on a sweep — so paging pays it once. Version 0 bypasses the cache.
type searchTotalsCache struct {
	mu      sync.Mutex
	version int64
	totals  map[searchKey]int64
}

type searchKey struct {
	q       string
	forumID int32
}

// maxCachedTotals bounds the map; distinct (q, forum) pairs accumulate from
// type-ahead typing. Dump-and-restart eviction is fine at this size — the
// entries are 16 bytes plus the query string.
const maxCachedTotals = 4096

func (c *searchTotalsCache) get(version int64, key searchKey) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if version == 0 || c.version != version {
		return 0, false
	}
	v, ok := c.totals[key]
	return v, ok
}

func (c *searchTotalsCache) set(version int64, key searchKey, total int64) {
	if version == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version != version || c.totals == nil || len(c.totals) >= maxCachedTotals {
		if version < c.version {
			return // stale writer from before a sweep landed
		}
		c.version = version
		c.totals = make(map[searchKey]int64)
	}
	c.totals[key] = total
}

func searchHandler(pool *pgxpool.Pool) http.HandlerFunc {
	totals := &searchTotalsCache{}
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		params := store.SearchParams{
			Query:   q.Get("q"),
			ForumID: parseInt32(q.Get("forum_id"), 0),
			Sort:    q.Get("sort"),
			Dir:     q.Get("dir"),
			Offset:  parseInt(q.Get("offset"), 0),
			Limit:   parseInt(q.Get("limit"), 50),
		}

		items, err := store.Search(r.Context(), pool, params)
		if err != nil {
			slog.Error("search", "err", err, "q", params.Query)
			writeJSON(w, http.StatusInternalServerError, errResp("search failed"))
			return
		}

		version := latestParserVersion(r.Context(), pool)
		key := searchKey{q: params.Query, forumID: params.ForumID}
		total, ok := totals.get(version, key)
		if !ok {
			total, err = store.CountSearch(r.Context(), pool, params.Query, params.ForumID)
			if err != nil {
				slog.Error("search count", "err", err, "q", params.Query)
				writeJSON(w, http.StatusInternalServerError, errResp("search failed"))
				return
			}
			totals.set(version, key, total)
		}

		writeJSON(w, http.StatusOK, store.SearchResult{Items: items, Total: total})
	}
}

// torrentDetail extends store.Torrent for the single-torrent response with
// a rendered HTML body. We don't expose the raw BBCode by default.
type torrentDetail struct {
	store.Torrent
	ContentHTML string `json:"content_html"`
}

func torrentHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, errResp("invalid id"))
			return
		}
		t, err := store.Get(r.Context(), pool, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, errResp("not found"))
				return
			}
			slog.Error("torrent get", "err", err, "id", id)
			writeJSON(w, http.StatusInternalServerError, errResp("get failed"))
			return
		}
		writeJSON(w, http.StatusOK, torrentDetail{
			Torrent:     t,
			ContentHTML: bbcode.Render(t.Content),
		})
	}
}

// maxResponseFiles caps how many entries one listing response carries. The
// dump's median torrent has 10 files and its p99 has 429, but the tail reaches
// 33k — enough JSON to stall the drawer. files_count keeps the true number, so
// the UI can say how many were left out.
const maxResponseFiles = 1000

// filesHandler serves a torrent's file listing. The stored blob decompresses
// straight into the response body — the common case never parses JSON; only an
// oversized listing is decoded, cut and re-encoded.
func filesHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseTorrentID(w, r)
		if !ok {
			return
		}
		// Listings are immutable between dumps, so the parser_run id is an
		// exact validator — a reopened card revalidates into a 304.
		if applyVersionETag(w, r, latestParserVersion(r.Context(), pool)) {
			return
		}
		blob, count, err := store.GetFiles(r.Context(), pool, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, errResp("not found"))
				return
			}
			slog.Error("torrent files", "err", err, "id", id)
			writeJSON(w, http.StatusInternalServerError, errResp("files failed"))
			return
		}

		truncated := count > maxResponseFiles
		var items []byte
		if truncated {
			files, decErr := filelist.Decode(blob)
			if decErr == nil {
				items = filelist.EncodeJSON(files[:maxResponseFiles])
			}
			err = decErr
		} else {
			items, err = filelist.DecodeJSON(blob)
		}
		if err != nil {
			slog.Error("torrent files decode", "err", err, "id", id)
			writeJSON(w, http.StatusInternalServerError, errResp("files failed"))
			return
		}

		// Assembled by hand so the stored JSON array goes out verbatim.
		head := fmt.Appendf(nil, `{"files_count":%d,"truncated":%t,"files":`, count, truncated)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(head)
		_, _ = w.Write(items)
		_, _ = w.Write([]byte("}"))
	}
}

// Favorites — global shared list; no auth, no per-user scoping. Add/remove
// are idempotent so the client can fire-and-forget after an optimistic update.

func listFavoritesHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListFavorites(r.Context(), pool)
		if err != nil {
			slog.Error("favorites list", "err", err)
			writeJSON(w, http.StatusInternalServerError, errResp("favorites failed"))
			return
		}
		if items == nil {
			items = []store.FavoriteItem{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func addFavoriteHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseTorrentID(w, r)
		if !ok {
			return
		}
		if err := store.AddFavorite(r.Context(), pool, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, errResp("not found"))
				return
			}
			slog.Error("favorites add", "err", err, "id", id)
			writeJSON(w, http.StatusInternalServerError, errResp("add favorite failed"))
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	}
}

func removeFavoriteHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseTorrentID(w, r)
		if !ok {
			return
		}
		if err := store.RemoveFavorite(r.Context(), pool, id); err != nil {
			slog.Error("favorites remove", "err", err, "id", id)
			writeJSON(w, http.StatusInternalServerError, errResp("remove favorite failed"))
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	}
}

func clearFavoritesHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.ClearFavorites(r.Context(), pool); err != nil {
			slog.Error("favorites clear", "err", err)
			writeJSON(w, http.StatusInternalServerError, errResp("clear favorites failed"))
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	}
}

func parseTorrentID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, errResp("invalid id"))
		return 0, false
	}
	return id, true
}

// --- small parsing helpers (no library; this is the entire usage) -----------

func parseInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func parseInt32(s string, fallback int32) int32 {
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < 0 {
		return fallback
	}
	return int32(v)
}

func errResp(msg string) map[string]string { return map[string]string{"error": msg} }
