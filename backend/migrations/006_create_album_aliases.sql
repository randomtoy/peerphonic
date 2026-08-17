CREATE TABLE album_alias_tracks (
    provider  TEXT NOT NULL,
    alias_id  TEXT NOT NULL,
    track_id  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    PRIMARY KEY (provider, alias_id, track_id)
);

CREATE INDEX album_alias_tracks_lookup_idx ON album_alias_tracks(alias_id, track_id);
