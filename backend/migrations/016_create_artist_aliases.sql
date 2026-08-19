CREATE TABLE artist_aliases (
    alias_id     TEXT PRIMARY KEY,
    alias_name   TEXT NOT NULL,
    target_id    TEXT NOT NULL,
    target_name  TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    CHECK (alias_id <> target_id)
);

CREATE INDEX artist_aliases_target_idx ON artist_aliases(target_id, alias_name COLLATE NOCASE);
