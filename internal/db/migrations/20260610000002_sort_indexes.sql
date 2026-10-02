-- +goose Up

-- Indexes backing the sort orders /api/search actually offers. With the
-- window-function count gone (the page query is a pure index-ordered
-- top-N now), missing sort indexes became the dominant per-request cost:
--
--   sort=size, no filter      → sorted all ~2.8M rows per request
--   forum filter + sort=date  → fetched the whole forum via
--                               torrents_forum_id_idx, then sorted it
--   forum filter + sort=size  → same, sorted by size
--
-- (registered_at DESC already exists from init for the unfiltered browse.)
-- Cost: ~60-80 MB disk each + write amplification once a month during the
-- ingest — cheap against per-click sorts of millions of rows.
--
-- Plain CREATE INDEX (not CONCURRENTLY): migrations run at boot, when the
-- old container is already gone and nothing writes to the table. Adds a
-- one-off ~1-2 min to the first boot after this deploy.
CREATE INDEX torrents_size_idx          ON torrents (size_bytes);
CREATE INDEX torrents_forum_date_idx    ON torrents (forum_id, registered_at DESC);
CREATE INDEX torrents_forum_size_idx    ON torrents (forum_id, size_bytes);

-- The single-column forum_id index is now a strict prefix of the two
-- composites — redundant for every query shape we run.
DROP INDEX IF EXISTS torrents_forum_id_idx;

-- +goose Down

CREATE INDEX torrents_forum_id_idx ON torrents (forum_id);
DROP INDEX IF EXISTS torrents_forum_size_idx;
DROP INDEX IF EXISTS torrents_forum_date_idx;
DROP INDEX IF EXISTS torrents_size_idx;
