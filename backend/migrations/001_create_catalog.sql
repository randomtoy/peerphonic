CREATE TABLE tracks (
    id            TEXT PRIMARY KEY,
    provider      TEXT NOT NULL,
    source_key    TEXT NOT NULL,
    title         TEXT NOT NULL,
    artist        TEXT NOT NULL,
    artist_id     TEXT NOT NULL,
    album         TEXT NOT NULL,
    album_id      TEXT NOT NULL,
    album_artist  TEXT NOT NULL,
    track_number  INTEGER NOT NULL DEFAULT 0,
    disc_number   INTEGER NOT NULL DEFAULT 0,
    year          INTEGER NOT NULL DEFAULT 0,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    bit_rate      INTEGER NOT NULL DEFAULT 0,
    suffix        TEXT NOT NULL,
    content_type  TEXT NOT NULL,
    UNIQUE(provider, source_key)
);

CREATE INDEX tracks_provider_idx ON tracks(provider);
CREATE INDEX tracks_artist_idx ON tracks(artist_id, album_artist, album);
CREATE INDEX tracks_album_idx ON tracks(album_id, disc_number, track_number, title);
