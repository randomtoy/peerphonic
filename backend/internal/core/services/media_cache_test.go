package services

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/blob/filesystem"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestMediaCacheEvictsLeastRecentlyUsedUnpinnedEntry(t *testing.T) {
	t.Parallel()

	cache := newTestMediaCache(t, 8)
	if err := cache.Put(context.Background(), "first", strings.NewReader("1111"), false); err != nil {
		t.Fatal(err)
	}
	if err := cache.Put(context.Background(), "second", strings.NewReader("2222"), false); err != nil {
		t.Fatal(err)
	}
	content, err := cache.Open(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	content.Close()
	if err := cache.Put(context.Background(), "third", strings.NewReader("3333"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Open(context.Background(), "second"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Open(second) error = %v, want ErrNotFound", err)
	}
	assertCachedContent(t, cache, "first", "1111")
	assertCachedContent(t, cache, "third", "3333")

	if err := cache.SetPinned(context.Background(), "first", true); err != nil {
		t.Fatal(err)
	}
	if err := cache.Put(context.Background(), "fourth", strings.NewReader("4444"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Open(context.Background(), "third"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Open(third) error = %v, want ErrNotFound", err)
	}
	stats, err := cache.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 2 || stats.Size != 8 || stats.PinnedEntries != 1 || stats.PinnedSize != 4 {
		t.Fatalf("Stats() = %#v", stats)
	}
}

func TestMediaCacheKeepsPinnedMediaBeyondLimit(t *testing.T) {
	t.Parallel()

	cache := newTestMediaCache(t, 4)
	if err := cache.Put(context.Background(), "pinned", strings.NewReader("123456"), true); err != nil {
		t.Fatalf("Put(pinned) error = %v", err)
	}
	err := cache.Put(context.Background(), "extra", strings.NewReader("x"), false)
	if !errors.Is(err, ErrCacheCapacity) {
		t.Fatalf("Put(extra) error = %v, want ErrCacheCapacity", err)
	}
	assertCachedContent(t, cache, "pinned", "123456")
	if _, err := cache.Open(context.Background(), "extra"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Open(extra) error = %v, want ErrNotFound", err)
	}
}

func newTestMediaCache(t *testing.T, maxBytes int64) *MediaCache {
	t.Helper()
	ctx := context.Background()
	metadata, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { metadata.Close() })
	blobs, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cache, err := NewMediaCache(blobs, metadata, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 17, 0, 0, 0, 0, time.UTC)
	cache.now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	return cache
}

func assertCachedContent(t *testing.T, cache *MediaCache, key, want string) {
	t.Helper()
	content, err := cache.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("Open(%s) error = %v", key, err)
	}
	defer content.Close()
	got, err := io.ReadAll(content)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("Open(%s) = %q, want %q", key, got, want)
	}
}
