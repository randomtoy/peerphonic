package services

import (
	"context"
	"errors"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type cacheStatsSourceStub struct {
	stats domain.CacheStats
	err   error
}

func (s cacheStatsSourceStub) Stats(context.Context) (domain.CacheStats, error) {
	return s.stats, s.err
}

type cacheUsageSourceStub struct {
	usage domain.CacheUsage
	err   error
}

func (s cacheUsageSourceStub) CacheUsage(context.Context) (domain.CacheUsage, error) {
	return s.usage, s.err
}

func TestCacheStatusCombinesProviderManagedUsage(t *testing.T) {
	t.Parallel()

	status := NewCacheStatus(
		cacheStatsSourceStub{stats: domain.CacheStats{
			Capacity: 100, Size: 20, Entries: 1,
			Components: []domain.CacheUsage{{Name: "media", Size: 20, Entries: 1}},
		}},
		cacheUsageSourceStub{usage: domain.CacheUsage{
			Name: "torrent", Size: 30, Entries: 4, PartialEntries: 3,
		}},
	)
	stats, err := status.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Capacity != 100 || stats.Size != 50 || stats.Entries != 5 ||
		len(stats.Components) != 2 || stats.Components[1].PartialEntries != 3 {
		t.Fatalf("Stats() = %#v", stats)
	}
}

func TestCacheStatusReturnsProviderUsageError(t *testing.T) {
	t.Parallel()

	want := errors.New("unavailable")
	status := NewCacheStatus(cacheStatsSourceStub{}, cacheUsageSourceStub{err: want})
	_, err := status.Stats(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Stats() error = %v, want %v", err, want)
	}
}
