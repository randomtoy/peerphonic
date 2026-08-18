package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

var ErrCacheCapacity = errors.New("media cache capacity exceeded")

type MediaCache struct {
	blobs     ports.BlobStore
	metadata  ports.CacheMetadataStore
	maxBytes  int64
	now       func() time.Time
	operation sync.Mutex
}

func NewMediaCache(blobs ports.BlobStore, metadata ports.CacheMetadataStore, maxBytes int64) (*MediaCache, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("media cache size must be positive")
	}
	return &MediaCache{
		blobs: blobs, metadata: metadata, maxBytes: maxBytes, now: time.Now,
	}, nil
}

func (c *MediaCache) Put(ctx context.Context, key string, source io.Reader, pinned bool) error {
	if !validCacheKey(key) {
		return fmt.Errorf("invalid media cache key %q", key)
	}
	counter := &countingReader{reader: source}
	if err := c.blobs.Put(ctx, mediaCacheBlobKey(key), counter); err != nil {
		return fmt.Errorf("store cached media %q: %w", key, err)
	}

	c.operation.Lock()
	defer c.operation.Unlock()
	entry := domain.CacheEntry{
		Key: key, Size: counter.bytes, LastAccessed: c.now().UTC(), Pinned: pinned,
	}
	if err := c.metadata.SaveCacheEntry(ctx, entry); err != nil {
		_ = c.blobs.Delete(context.WithoutCancel(ctx), mediaCacheBlobKey(key))
		return fmt.Errorf("record cached media %q: %w", key, err)
	}
	remaining, err := c.pruneLocked(ctx, key)
	if err != nil {
		return err
	}
	if remaining > c.maxBytes && !pinned {
		if err := c.removeLocked(ctx, key); err != nil {
			return fmt.Errorf("discard over-capacity media %q: %w", key, err)
		}
		return fmt.Errorf("cache media %q: %w", key, ErrCacheCapacity)
	}
	return nil
}

func (c *MediaCache) Open(ctx context.Context, key string) (ports.ReadSeekCloser, error) {
	c.operation.Lock()
	defer c.operation.Unlock()

	entry, err := c.metadata.CacheEntry(ctx, key)
	if err != nil {
		return nil, err
	}
	content, err := c.blobs.Open(ctx, mediaCacheBlobKey(key))
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			_ = c.metadata.DeleteCacheEntry(context.WithoutCancel(ctx), key)
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("open cached media %q: %w", key, err)
	}
	entry.LastAccessed = c.now().UTC()
	if err := c.metadata.SaveCacheEntry(ctx, entry); err != nil {
		content.Close()
		return nil, fmt.Errorf("touch cached media %q: %w", key, err)
	}
	return content, nil
}

func (c *MediaCache) SetPinned(ctx context.Context, key string, pinned bool) error {
	c.operation.Lock()
	defer c.operation.Unlock()

	entry, err := c.metadata.CacheEntry(ctx, key)
	if err != nil {
		return err
	}
	entry.Pinned = pinned
	if err := c.metadata.SaveCacheEntry(ctx, entry); err != nil {
		return fmt.Errorf("set cached media pin %q: %w", key, err)
	}
	if !pinned {
		_, err = c.pruneLocked(ctx, "")
		return err
	}
	return nil
}

func (c *MediaCache) Remove(ctx context.Context, key string) error {
	c.operation.Lock()
	defer c.operation.Unlock()
	return c.removeLocked(ctx, key)
}

func (c *MediaCache) Prune(ctx context.Context) error {
	c.operation.Lock()
	defer c.operation.Unlock()
	_, err := c.pruneLocked(ctx, "")
	return err
}

func (c *MediaCache) Stats(ctx context.Context) (domain.CacheStats, error) {
	c.operation.Lock()
	defer c.operation.Unlock()

	entries, err := c.metadata.CacheEntries(ctx)
	if err != nil {
		return domain.CacheStats{}, err
	}
	stats := domain.CacheStats{Capacity: c.maxBytes}
	stats.Entries = len(entries)
	for _, entry := range entries {
		stats.Size += entry.Size
		if entry.Pinned {
			stats.PinnedEntries++
			stats.PinnedSize += entry.Size
		}
	}
	stats.Components = append(stats.Components, domain.CacheUsage{
		Name: "media", Entries: stats.Entries, Size: stats.Size,
	})
	return stats, nil
}

func (c *MediaCache) pruneLocked(ctx context.Context, protectedKey string) (int64, error) {
	entries, err := c.metadata.CacheEntries(ctx)
	if err != nil {
		return 0, fmt.Errorf("list cached media: %w", err)
	}
	var size int64
	for _, entry := range entries {
		size += entry.Size
	}
	for _, entry := range entries {
		if size <= c.maxBytes {
			break
		}
		if entry.Pinned || entry.Key == protectedKey {
			continue
		}
		if err := c.removeLocked(ctx, entry.Key); err != nil {
			return size, fmt.Errorf("evict cached media %q: %w", entry.Key, err)
		}
		size -= entry.Size
	}
	return size, nil
}

func (c *MediaCache) removeLocked(ctx context.Context, key string) error {
	err := c.blobs.Delete(ctx, mediaCacheBlobKey(key))
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("delete cached media bytes: %w", err)
	}
	if err := c.metadata.DeleteCacheEntry(ctx, key); err != nil {
		return fmt.Errorf("delete cached media metadata: %w", err)
	}
	return nil
}

func validCacheKey(key string) bool {
	return key != "" && !strings.ContainsAny(key, `/\\`)
}

func mediaCacheBlobKey(key string) string {
	return "media/" + key
}

type countingReader struct {
	reader io.Reader
	bytes  int64
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	read, err := r.reader.Read(buffer)
	r.bytes += int64(read)
	return read, err
}
