package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FavoriteItem is the public shape of a favorited torrent. It piggy-backs on
// Torrent for the card fields the UI already renders, and adds AddedAt so the
// favorites view can sort "newest first" without a separate fetch.
type FavoriteItem struct {
	Torrent
	AddedAt time.Time `json:"added_at"`
}

// ListFavorites returns every favorited torrent joined with its current row
// in `torrents`. CASCADE on the favorites FK guarantees swept torrents
// don't show up here as dangling rows. Ordered by added_at DESC so the UI
// can render directly without an extra sort.
func ListFavorites(ctx context.Context, pool *pgxpool.Pool) ([]FavoriteItem, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.id, t.title, t.forum_id, t.forum_name,
		       t.size_bytes, t.registered_at, t.hash,
		       tp.seeders, tp.leechers, tp.checked_at,
		       f.added_at
		FROM favorites f
		JOIN torrents       t  ON t.id = f.torrent_id
		LEFT JOIN torrent_peers tp ON tp.torrent_id = t.id
		ORDER BY f.added_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()

	out := make([]FavoriteItem, 0)
	for rows.Next() {
		var it FavoriteItem
		if err := rows.Scan(
			&it.ID, &it.Title, &it.ForumID, &it.ForumName,
			&it.SizeBytes, &it.RegisteredAt, &it.Hash,
			&it.Seeders, &it.Leechers, &it.PeersCheckedAt,
			&it.AddedAt,
		); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	return out, nil
}

// AddFavorite is idempotent — a re-star of an already-starred id is a no-op.
// Returns ErrNotFound when the torrent id doesn't exist; the FK violation
// from postgres is the round trip we'd otherwise spend on a precheck SELECT.
func AddFavorite(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO favorites (torrent_id) VALUES ($1)
		ON CONFLICT (torrent_id) DO NOTHING
	`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrNotFound
		}
		return fmt.Errorf("add favorite %d: %w", id, err)
	}
	return nil
}

// RemoveFavorite is idempotent — unstarring a non-starred id is a no-op.
func RemoveFavorite(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	_, err := pool.Exec(ctx, `DELETE FROM favorites WHERE torrent_id = $1`, id)
	if err != nil {
		return fmt.Errorf("remove favorite %d: %w", id, err)
	}
	return nil
}

// ClearFavorites empties the table. Used by the "Очистить избранное" button.
func ClearFavorites(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `DELETE FROM favorites`)
	if err != nil {
		return fmt.Errorf("clear favorites: %w", err)
	}
	return nil
}
