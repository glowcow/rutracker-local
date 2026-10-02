-- +goose Up

-- torrent_peers is the live seeders/leechers layer, kept OFF the cold
-- torrents heap (which the dump bulk-UPSERTs) so the peer refresher never
-- amplifies writes on the big table. One row per torrent, current snapshot
-- only (no history). checked_at drives the 24h refresh TTL. FK + CASCADE so a
-- dump sweep that deletes a vanished torrent takes its peer row with it.
CREATE TABLE torrent_peers (
    torrent_id BIGINT PRIMARY KEY REFERENCES torrents(id) ON DELETE CASCADE,
    seeders    INT NOT NULL,
    leechers   INT NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Supports a future "sort by seeders" over the catalogue without a table scan.
CREATE INDEX torrent_peers_seeders_idx ON torrent_peers (seeders DESC);

-- rutracker_session caches the logged-in bb_session cookie so we don't
-- re-login on every scrape. Single-row table: the CHECK pins id=TRUE, so the
-- PK admits exactly one row that we UPSERT. Cookie is minted from creds in
-- .rutracker.env (server-only) and re-minted on expiry.
CREATE TABLE rutracker_session (
    id          BOOLEAN PRIMARY KEY DEFAULT TRUE,
    cookie      TEXT NOT NULL,
    obtained_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT rutracker_session_singleton CHECK (id)
);

-- +goose Down

DROP TABLE IF EXISTS rutracker_session;
DROP TABLE IF EXISTS torrent_peers;
