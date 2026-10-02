-- +goose Up

-- Single global favorites list — the service is shared with no per-user
-- accounts, so favorites are intentionally a single set, not per-user. The
-- only column besides the torrent_id is added_at, used to sort the
-- favorites view "newest starred first" by default.
--
-- ON DELETE CASCADE on the torrent_id FK piggy-backs on the parser sweep:
-- when a swept torrent disappears from the torrents table, its favorite
-- entry vanishes too, so the UI never lists a starred row whose detail
-- page would 404.
CREATE TABLE favorites (
    torrent_id BIGINT      PRIMARY KEY REFERENCES torrents(id) ON DELETE CASCADE,
    added_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Listing favorites is always "newest first" — a B-tree on added_at DESC is
-- the smallest index that serves both default-sort listing and the count.
CREATE INDEX favorites_added_at_idx ON favorites (added_at DESC);

-- +goose Down

DROP INDEX IF EXISTS favorites_added_at_idx;
DROP TABLE IF EXISTS favorites;
