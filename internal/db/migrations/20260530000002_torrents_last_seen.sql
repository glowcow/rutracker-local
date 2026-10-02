-- +goose Up

-- last_seen_at records the wall-clock instant a torrent was touched by the
-- most recent parse run. The parser captures `SELECT NOW()` BEFORE walking
-- the dump and, after the run, drops any row whose last_seen_at is older
-- than that cutoff — i.e., entries that vanished from the new dump.
--
-- DEFAULT NOW() backfills the existing rows to the migration instant; the
-- first parse run after this migration sees them as "stale before me" only
-- for IDs the parser doesn't re-emit, so the first sweep naturally garbage-
-- collects deletions accumulated up to this point.
ALTER TABLE torrents ADD COLUMN last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Sweep is the only query that scans this column and it's range-based; a
-- plain B-tree index is the right shape and small enough not to hurt the
-- write path.
CREATE INDEX torrents_last_seen_at_idx ON torrents (last_seen_at);

-- +goose Down

DROP INDEX IF EXISTS torrents_last_seen_at_idx;
ALTER TABLE torrents DROP COLUMN IF EXISTS last_seen_at;
