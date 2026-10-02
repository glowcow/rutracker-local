-- +goose Up

-- torrent_files is the per-torrent file listing carried by the dump (<dir>/
-- <file> subtree), kept OFF the torrents heap: the drawer reads it once per
-- opened card, while search never touches it. FK + CASCADE so a sweep that
-- drops a vanished torrent takes its listing along.
-- files is a deflate-compressed JSON array of [path, size] pairs (see
-- internal/filelist); files_count is the true count, kept readable without
-- decompressing. STORAGE EXTERNAL: the payload is already compressed, so
-- letting TOAST retry pglz on it only burns CPU.
CREATE TABLE torrent_files (
    torrent_id  BIGINT PRIMARY KEY REFERENCES torrents(id) ON DELETE CASCADE,
    files_count INT    NOT NULL,
    files       BYTEA  NOT NULL
);

ALTER TABLE torrent_files ALTER COLUMN files SET STORAGE EXTERNAL;

-- +goose Down

DROP TABLE IF EXISTS torrent_files;
