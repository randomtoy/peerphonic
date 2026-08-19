CREATE TABLE track_aliases (
    alias_id  TEXT PRIMARY KEY,
    track_id  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE
);

CREATE INDEX track_aliases_track_idx ON track_aliases(track_id);
