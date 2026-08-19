CREATE TABLE play_queues (
    owner             TEXT PRIMARY KEY,
    current_track_id  TEXT NOT NULL DEFAULT '',
    position_ms       INTEGER NOT NULL DEFAULT 0 CHECK (position_ms >= 0),
    changed_at        TEXT NOT NULL,
    changed_by        TEXT NOT NULL DEFAULT ''
);

CREATE TABLE play_queue_tracks (
    owner     TEXT NOT NULL REFERENCES play_queues(owner) ON DELETE CASCADE,
    position  INTEGER NOT NULL,
    track_id  TEXT NOT NULL,
    PRIMARY KEY (owner, position)
);

CREATE INDEX play_queue_tracks_track_idx ON play_queue_tracks(track_id);
