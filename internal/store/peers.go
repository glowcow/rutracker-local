package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Peers is a torrent's current cached peer snapshot.
type Peers struct {
	Seeders   int
	Leechers  int
	CheckedAt time.Time
}

// UpsertPeers writes (or refreshes) the peer counts for one torrent, stamping
// checked_at = NOW(). Called after a successful scrape.
func UpsertPeers(ctx context.Context, pool *pgxpool.Pool, torrentID int64, seeders, leechers int) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO torrent_peers (torrent_id, seeders, leechers, checked_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (torrent_id) DO UPDATE SET
			seeders    = EXCLUDED.seeders,
			leechers   = EXCLUDED.leechers,
			checked_at = EXCLUDED.checked_at
	`, torrentID, seeders, leechers)
	if err != nil {
		return fmt.Errorf("upsert peers %d: %w", torrentID, err)
	}
	return nil
}

// GetPeers returns the cached snapshot for a torrent. The bool is false (with
// nil error) when no row exists yet.
func GetPeers(ctx context.Context, pool *pgxpool.Pool, torrentID int64) (Peers, bool, error) {
	var p Peers
	err := pool.QueryRow(ctx, `
		SELECT seeders, leechers, checked_at
		FROM torrent_peers WHERE torrent_id = $1
	`, torrentID).Scan(&p.Seeders, &p.Leechers, &p.CheckedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Peers{}, false, nil
	}
	if err != nil {
		return Peers{}, false, fmt.Errorf("get peers %d: %w", torrentID, err)
	}
	return p, true, nil
}

// CountCachedPeers returns how many torrents have a cached peer snapshot.
// Cheap (small table); computed fresh per request so the dashboard number
// grows as browsing populates the cache.
func CountCachedPeers(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM torrent_peers`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count cached peers: %w", err)
	}
	return n, nil
}

// TorrentExists reports whether an id is in the catalogue — a cheap guard so
// the peers endpoint never scrapes rutracker for ids that aren't ours.
func TorrentExists(ctx context.Context, pool *pgxpool.Pool, torrentID int64) (bool, error) {
	var one int
	err := pool.QueryRow(ctx, `SELECT 1 FROM torrents WHERE id = $1`, torrentID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("torrent exists %d: %w", torrentID, err)
	}
	return true, nil
}

// LoadRutrackerSession returns the persisted bb_session cookie (bool false
// when none has been stored yet).
func LoadRutrackerSession(ctx context.Context, pool *pgxpool.Pool) (string, bool, error) {
	var cookie string
	err := pool.QueryRow(ctx, `SELECT cookie FROM rutracker_session WHERE id = TRUE`).Scan(&cookie)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load rutracker session: %w", err)
	}
	return cookie, true, nil
}

// SaveRutrackerSession upserts the single session row with a fresh cookie.
func SaveRutrackerSession(ctx context.Context, pool *pgxpool.Pool, cookie string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO rutracker_session (id, cookie, obtained_at)
		VALUES (TRUE, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET
			cookie      = EXCLUDED.cookie,
			obtained_at = EXCLUDED.obtained_at
	`, cookie)
	if err != nil {
		return fmt.Errorf("save rutracker session: %w", err)
	}
	return nil
}
