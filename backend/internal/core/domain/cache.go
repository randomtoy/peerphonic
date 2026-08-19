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
	Capacity      int64
	Entries       int
	Size          int64
	PinnedEntries int
	PinnedSize    int64
	Components    []CacheUsage
}

// CacheUsage describes the storage occupied by one cache implementation.
// PartialEntries are entries whose bytes are not yet fully available.
type CacheUsage struct {
	Name           string
	Entries        int
	PartialEntries int
	Size           int64
}
