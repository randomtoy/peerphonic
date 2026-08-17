ALTER TABLE tracks ADD COLUMN genre TEXT NOT NULL DEFAULT '';

CREATE INDEX tracks_genre_idx ON tracks(genre COLLATE NOCASE, album_id);
