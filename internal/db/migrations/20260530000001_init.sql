-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE torrents (
    id            BIGINT PRIMARY KEY,
    title         TEXT        NOT NULL,
    forum_id      INT         NOT NULL,
    forum_name    TEXT        NOT NULL,
    size_bytes    BIGINT      NOT NULL,
    registered_at TIMESTAMPTZ NOT NULL,
    hash          CHAR(40)    NOT NULL,
    content       TEXT        NOT NULL,
    search_vec    tsvector
        GENERATED ALWAYS AS (
            setweight(to_tsvector('russian', coalesce(title, '')), 'A') ||
            setweight(to_tsvector('russian', coalesce(forum_name, '')), 'B')
        ) STORED
);

CREATE INDEX torrents_search_vec_idx     ON torrents USING GIN (search_vec);
CREATE INDEX torrents_title_trgm_idx     ON torrents USING GIN (title gin_trgm_ops);
CREATE INDEX torrents_registered_at_idx  ON torrents (registered_at DESC);
CREATE INDEX torrents_forum_id_idx       ON torrents (forum_id);

-- +goose Down

DROP TABLE IF EXISTS torrents;
-- Keep pg_trgm — other consumers may rely on it.
