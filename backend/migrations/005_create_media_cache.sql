CREATE TABLE media_cache_entries (
    key               TEXT PRIMARY KEY,
    size_bytes        INTEGER NOT NULL CHECK (size_bytes >= 0),
    last_accessed_at  TEXT NOT NULL,
    pinned            INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1))
);

CREATE INDEX media_cache_lru_idx
    ON media_cache_entries(pinned, last_accessed_at, key);
