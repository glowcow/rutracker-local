-- +goose Up

-- torrents_title_trgm_idx was created for ILIKE / similarity() fuzzy title
-- search that never materialised — every query path goes through the
-- tsvector GIN (search_vec @@ plainto_tsquery). A trigram GIN over ~2.8M
-- titles costs hundreds of MB of disk + buffer cache and write
-- amplification on every monthly ingest, all for zero reads. The pg_trgm
-- extension itself stays: it's free when unused, and ad-hoc psql sessions
-- can still use similarity() against a fresh index if the need returns.
DROP INDEX IF EXISTS torrents_title_trgm_idx;

-- +goose Down

CREATE INDEX torrents_title_trgm_idx ON torrents USING GIN (title gin_trgm_ops);
