DROP INDEX tracks_provider_idx;
DROP INDEX tracks_artist_idx;
DROP INDEX tracks_album_idx;
DROP INDEX tracks_album_artist_idx;

ALTER TABLE tracks RENAME TO tracks_legacy;

CREATE TABLE tracks (
    id               TEXT PRIMARY KEY,
    title            TEXT NOT NULL,
    artist           TEXT NOT NULL,
    artist_id        TEXT NOT NULL,
    album            TEXT NOT NULL,
    album_id         TEXT NOT NULL,
    album_artist     TEXT NOT NULL,
    album_artist_id  TEXT NOT NULL DEFAULT '',
    track_number     INTEGER NOT NULL DEFAULT 0,
    disc_number      INTEGER NOT NULL DEFAULT 0,
    year             INTEGER NOT NULL DEFAULT 0,
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    size_bytes       INTEGER NOT NULL DEFAULT 0,
    bit_rate         INTEGER NOT NULL DEFAULT 0,
    suffix           TEXT NOT NULL,
    content_type     TEXT NOT NULL,
    cover_art_id     TEXT NOT NULL DEFAULT ''
);

INSERT INTO tracks (
    id, title, artist, artist_id, album, album_id, album_artist,
    album_artist_id, track_number, disc_number, year, duration_ms,
    size_bytes, bit_rate, suffix, content_type, cover_art_id
)
SELECT
    id, title, artist, artist_id, album, album_id, album_artist,
    album_artist_id, track_number, disc_number, year, duration_ms,
    size_bytes, bit_rate, suffix, content_type, cover_art_id
FROM tracks_legacy;

CREATE TABLE track_sources (
    track_id    TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,
    source_key  TEXT NOT NULL,
    PRIMARY KEY (track_id, provider, source_key),
    UNIQUE (provider, source_key)
);

INSERT INTO track_sources (track_id, provider, source_key)
SELECT id, provider, source_key FROM tracks_legacy;

DROP TABLE tracks_legacy;

CREATE INDEX tracks_artist_idx ON tracks(artist_id, album_artist, album);
CREATE INDEX tracks_album_idx ON tracks(album_id, disc_number, track_number, title);
CREATE INDEX tracks_album_artist_idx ON tracks(album_artist_id, album_artist, album);
CREATE INDEX track_sources_track_idx ON track_sources(track_id);
CREATE INDEX track_sources_provider_idx ON track_sources(provider);
