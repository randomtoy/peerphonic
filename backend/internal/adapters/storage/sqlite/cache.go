package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func (c *Catalog) CacheEntry(ctx context.Context, key string) (domain.CacheEntry, error) {
	row := c.db.QueryRowContext(ctx, `SELECT key, size_bytes, last_accessed_at, pinned
		FROM media_cache_entries WHERE key = ?`, key)
	entry, err := scanCacheEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CacheEntry{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.CacheEntry{}, fmt.Errorf("query cache entry: %w", err)
	}
	return entry, nil
}

func (c *Catalog) CacheEntries(ctx context.Context) ([]domain.CacheEntry, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT key, size_bytes, last_accessed_at, pinned
		FROM media_cache_entries ORDER BY last_accessed_at, key`)
	if err != nil {
		return nil, fmt.Errorf("query cache entries: %w", err)
	}
	defer rows.Close()

	var entries []domain.CacheEntry
	for rows.Next() {
		entry, err := scanCacheEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cache entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cache entries: %w", err)
	}
	return entries, nil
}

func (c *Catalog) SaveCacheEntry(ctx context.Context, entry domain.CacheEntry) error {
	_, err := c.db.ExecContext(ctx, `INSERT INTO media_cache_entries (
		key, size_bytes, last_accessed_at, pinned
	) VALUES (?, ?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET
		size_bytes = excluded.size_bytes,
		last_accessed_at = excluded.last_accessed_at,
		pinned = excluded.pinned`,
		entry.Key, entry.Size, entry.LastAccessed.UTC().Format(time.RFC3339Nano), entry.Pinned,
	)
	if err != nil {
		return fmt.Errorf("save cache entry %q: %w", entry.Key, err)
	}
	return nil
}

func (c *Catalog) DeleteCacheEntry(ctx context.Context, key string) error {
	if _, err := c.db.ExecContext(ctx, "DELETE FROM media_cache_entries WHERE key = ?", key); err != nil {
		return fmt.Errorf("delete cache entry %q: %w", key, err)
	}
	return nil
}

func scanCacheEntry(row rowScanner) (domain.CacheEntry, error) {
	var entry domain.CacheEntry
	var lastAccessed string
	if err := row.Scan(&entry.Key, &entry.Size, &lastAccessed, &entry.Pinned); err != nil {
		return domain.CacheEntry{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, lastAccessed)
	if err != nil {
		return domain.CacheEntry{}, fmt.Errorf("parse last access time: %w", err)
	}
	entry.LastAccessed = parsed
	return entry, nil
}
