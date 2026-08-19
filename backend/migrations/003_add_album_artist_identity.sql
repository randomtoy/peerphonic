ALTER TABLE tracks ADD COLUMN album_artist_id TEXT NOT NULL DEFAULT '';

UPDATE tracks SET album_artist_id = artist_id;

CREATE INDEX tracks_album_artist_idx ON tracks(album_artist_id, album_artist, album);
