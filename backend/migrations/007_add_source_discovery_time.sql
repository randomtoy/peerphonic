ALTER TABLE track_sources ADD COLUMN discovered_at TEXT NOT NULL DEFAULT '';

CREATE INDEX track_sources_discovered_idx
    ON track_sources(discovered_at, track_id);
