CREATE TABLE playlists (
    id          TEXT PRIMARY KEY,
    owner       TEXT NOT NULL,
    name        TEXT NOT NULL,
    comment     TEXT NOT NULL DEFAULT '',
    public      INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    changed_at  TEXT NOT NULL
);

CREATE INDEX playlists_owner_idx ON playlists(owner, name COLLATE NOCASE);

CREATE TABLE playlist_tracks (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    track_id    TEXT NOT NULL,
    PRIMARY KEY (playlist_id, position)
);

CREATE INDEX playlist_tracks_track_idx ON playlist_tracks(track_id);
