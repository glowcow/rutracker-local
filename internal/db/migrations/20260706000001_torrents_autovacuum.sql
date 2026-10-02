-- +goose Up

-- The dump ingest bumps last_seen_at on every row each load (Sweep needs it to
-- drop vanished torrents), and last_seen_at is indexed → non-HOT updates that
-- spike dead tuples by ~a whole table's worth per load. Default autovacuum only
-- fires at 20% dead (scale_factor 0.2); tighten it for this table so the churn
-- is reclaimed ~4× sooner and the dead ratio stays green. analyze_scale_factor
-- matched so planner stats refresh after a load too. Baked into a migration so
-- a from-scratch database inherits the same reloptions.
ALTER TABLE torrents SET (
    autovacuum_vacuum_scale_factor = 0.05,
    autovacuum_analyze_scale_factor = 0.05
);

-- +goose Down
ALTER TABLE torrents RESET (
    autovacuum_vacuum_scale_factor,
    autovacuum_analyze_scale_factor
);
