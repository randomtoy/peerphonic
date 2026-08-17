package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type cacheStatsSource interface {
	Stats(ctx context.Context) (domain.CacheStats, error)
}

type cacheUsageSource interface {
	CacheUsage(ctx context.Context) (domain.CacheUsage, error)
}

type cacheEvictor interface {
	Evict(ctx context.Context, bytes int64) (int64, error)
}

// CacheStatus combines the shared media cache and provider-managed partial
// data into one provider-independent capacity view.
type CacheStatus struct {
	primary cacheStatsSource
	sources []cacheUsageSource
	pruneMu sync.Mutex
}

// Prune asks provider caches to release their least recently used data until
// the combined cache is back within the configured capacity.
func (s *CacheStatus) Prune(ctx context.Context) error {
	s.pruneMu.Lock()
	defer s.pruneMu.Unlock()

	stats, err := s.Stats(ctx)
	if err != nil {
		return err
	}
	remaining := stats.Size - stats.Capacity
	if remaining <= 0 {
		return nil
	}
	for _, source := range s.sources {
		evictor, ok := source.(cacheEvictor)
		if !ok {
			continue
		}
		freed, err := evictor.Evict(ctx, remaining)
		if err != nil {
			return fmt.Errorf("evict provider cache data: %w", err)
		}
		remaining -= freed
		if remaining <= 0 {
			return nil
		}
	}
	return nil
}

func NewCacheStatus(primary cacheStatsSource, sources ...cacheUsageSource) *CacheStatus {
	return &CacheStatus{primary: primary, sources: sources}
}

func (s *CacheStatus) Stats(ctx context.Context) (domain.CacheStats, error) {
	stats, err := s.primary.Stats(ctx)
	if err != nil {
		return domain.CacheStats{}, fmt.Errorf("read shared media cache usage: %w", err)
	}
	for _, source := range s.sources {
		usage, err := source.CacheUsage(ctx)
		if err != nil {
			return domain.CacheStats{}, fmt.Errorf("read provider cache usage: %w", err)
		}
		stats.Size += usage.Size
		stats.Entries += usage.Entries
		stats.Components = append(stats.Components, usage)
	}
	return stats, nil
}
