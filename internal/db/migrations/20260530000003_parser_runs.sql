-- +goose Up

-- parser_runs is the durable history of every parse invocation, used both
-- as an operational record and as the source for the parser-side metrics
-- the API exposes on /metrics. The parser inserts a row at start, updates
-- it at finish; the API reads the latest row to populate gauges.
CREATE TABLE parser_runs (
    id            BIGSERIAL PRIMARY KEY,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ,
    source        TEXT NOT NULL,
    rows_inserted BIGINT NOT NULL DEFAULT 0,
    rows_swept    BIGINT NOT NULL DEFAULT 0,
    sweep         BOOLEAN NOT NULL DEFAULT FALSE,
    -- 'running' | 'succeeded' | 'failed' — a crashed parser leaves a row
    -- in 'running' state; the dashboard treats that as "no clean recent
    -- finish" rather than success.
    status        TEXT NOT NULL DEFAULT 'running'
);

CREATE INDEX parser_runs_started_at_idx ON parser_runs (started_at DESC);

-- +goose Down

DROP TABLE IF EXISTS parser_runs;
