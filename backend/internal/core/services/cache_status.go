package services

import (
	"context"
	"fmt"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type cacheStatsSource interface {
	Stats(ctx context.Context) (domain.CacheStats, error)
}

type cacheUsageSource interface {
	CacheUsage(ctx context.Context) (domain.CacheUsage, error)
}

// CacheStatus combines the shared media cache and provider-managed partial
// data into one provider-independent capacity view.
type CacheStatus struct {
	primary cacheStatsSource
	sources []cacheUsageSource
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
