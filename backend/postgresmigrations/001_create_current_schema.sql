CREATE TABLE tracks (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    artist TEXT NOT NULL,
    artist_id TEXT NOT NULL,
    album TEXT NOT NULL,
    album_id TEXT NOT NULL,
    album_artist TEXT NOT NULL,
    album_artist_id TEXT NOT NULL DEFAULT '',
    track_number INTEGER NOT NULL DEFAULT 0,
    disc_number INTEGER NOT NULL DEFAULT 0,
    year INTEGER NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    bit_rate INTEGER NOT NULL DEFAULT 0,
    suffix TEXT NOT NULL,
    content_type TEXT NOT NULL,
    cover_art_id TEXT NOT NULL DEFAULT '',
    genre TEXT NOT NULL DEFAULT ''
);

CREATE INDEX tracks_artist_idx ON tracks(artist_id, album_artist, album);
CREATE INDEX tracks_album_idx ON tracks(album_id, disc_number, track_number, title);
CREATE INDEX tracks_album_artist_idx ON tracks(album_artist_id, album_artist, album);
CREATE INDEX tracks_genre_idx ON tracks(LOWER(genre), album_id);

CREATE TABLE track_sources (
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    source_key TEXT NOT NULL,
    discovered_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (track_id, provider, source_key),
    UNIQUE (provider, source_key)
);

CREATE INDEX track_sources_track_idx ON track_sources(track_id);
CREATE INDEX track_sources_provider_idx ON track_sources(provider);
CREATE INDEX track_sources_discovered_idx ON track_sources(discovered_at, track_id);

CREATE TABLE media_cache_entries (
    key TEXT PRIMARY KEY,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    last_accessed_at TEXT NOT NULL,
    pinned BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX media_cache_lru_idx ON media_cache_entries(pinned, last_accessed_at, key);

CREATE TABLE album_alias_tracks (
    provider TEXT NOT NULL,
    alias_id TEXT NOT NULL,
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    PRIMARY KEY (provider, alias_id, track_id)
);
CREATE INDEX album_alias_tracks_lookup_idx ON album_alias_tracks(alias_id, track_id);

CREATE TABLE playlists (
    id TEXT PRIMARY KEY,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    public BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TEXT NOT NULL,
    changed_at TEXT NOT NULL
);
CREATE INDEX playlists_owner_idx ON playlists(owner, LOWER(name));

CREATE TABLE playlist_tracks (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    track_id TEXT NOT NULL,
    PRIMARY KEY (playlist_id, position)
);
CREATE INDEX playlist_tracks_track_idx ON playlist_tracks(track_id);

CREATE TABLE media_annotations (
    owner TEXT NOT NULL,
    media_type TEXT NOT NULL CHECK (media_type IN ('song', 'album', 'artist')),
    media_id TEXT NOT NULL,
    starred_at TEXT NOT NULL DEFAULT '',
    rating INTEGER NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
    play_count BIGINT NOT NULL DEFAULT 0 CHECK (play_count >= 0),
    last_played_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (owner, media_type, media_id)
);
CREATE INDEX media_annotations_starred_idx ON media_annotations(owner, starred_at DESC) WHERE starred_at <> '';

CREATE TABLE play_queues (
    owner TEXT PRIMARY KEY,
    current_track_id TEXT NOT NULL DEFAULT '',
    position_ms BIGINT NOT NULL DEFAULT 0 CHECK (position_ms >= 0),
    changed_at TEXT NOT NULL,
    changed_by TEXT NOT NULL DEFAULT ''
);
CREATE TABLE play_queue_tracks (
    owner TEXT NOT NULL REFERENCES play_queues(owner) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    track_id TEXT NOT NULL,
    PRIMARY KEY (owner, position)
);
CREATE INDEX play_queue_tracks_track_idx ON play_queue_tracks(track_id);

CREATE TABLE users (
    username TEXT PRIMARY KEY,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    password_hash BYTEA NOT NULL,
    encrypted_token BYTEA NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE user_permissions (
    username TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (username, permission)
);

CREATE TABLE torrent_transfer_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    upload_limit_bytes_per_second BIGINT NOT NULL CHECK (upload_limit_bytes_per_second >= 0),
    download_limit_bytes_per_second BIGINT NOT NULL CHECK (download_limit_bytes_per_second >= 0),
    updated_at TEXT NOT NULL
);

CREATE TABLE track_aliases (
    alias_id TEXT PRIMARY KEY,
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE
);
CREATE INDEX track_aliases_track_idx ON track_aliases(track_id);

CREATE TABLE artist_aliases (
    alias_id TEXT PRIMARY KEY,
    alias_name TEXT NOT NULL,
    target_id TEXT NOT NULL,
    target_name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    CHECK (alias_id <> target_id)
);
CREATE INDEX artist_aliases_target_idx ON artist_aliases(target_id, LOWER(alias_name));

CREATE TABLE pinned_tracks (track_id TEXT PRIMARY KEY, pinned_at TEXT NOT NULL);

CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TEXT NOT NULL,
    request_id TEXT NOT NULL,
    actor TEXT NOT NULL,
    remote_address TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status INTEGER NOT NULL
);
CREATE INDEX audit_log_occurred_at_idx ON audit_log(occurred_at DESC, id DESC);

CREATE OR REPLACE FUNCTION trim_audit_log() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.id % 100 = 0 THEN
        DELETE FROM audit_log WHERE id IN (
            SELECT id FROM audit_log ORDER BY occurred_at DESC, id DESC OFFSET 10000
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_retention AFTER INSERT ON audit_log
FOR EACH ROW EXECUTE FUNCTION trim_audit_log();
