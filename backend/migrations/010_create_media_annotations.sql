CREATE TABLE media_annotations (
    owner           TEXT NOT NULL,
    media_type      TEXT NOT NULL CHECK (media_type IN ('song', 'album', 'artist')),
    media_id        TEXT NOT NULL,
    starred_at      TEXT NOT NULL DEFAULT '',
    rating          INTEGER NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
    play_count      INTEGER NOT NULL DEFAULT 0 CHECK (play_count >= 0),
    last_played_at  TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (owner, media_type, media_id)
);

CREATE INDEX media_annotations_starred_idx
    ON media_annotations(owner, starred_at DESC)
    WHERE starred_at <> '';
