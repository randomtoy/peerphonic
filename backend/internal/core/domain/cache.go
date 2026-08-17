package domain

import "time"

// CacheEntry describes provider-independent media stored in the shared cache.
type CacheEntry struct {
	Key          string
	Size         int64
	LastAccessed time.Time
	Pinned       bool
}

type CacheStats struct {
	Entries       int
	Size          int64
	PinnedEntries int
	PinnedSize    int64
}
