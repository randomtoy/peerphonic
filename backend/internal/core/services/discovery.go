package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

var ErrDiscoveryResultNotFound = errors.New("discovery result not found")

const discoveryRetention = 30 * time.Minute

type DiscoveryService struct {
	provider ports.SourceSearcher
	catalog  ports.TrackSourceWriter

	mu      sync.Mutex
	results map[string]domain.TrackSource
}

func NewDiscoveryService(provider ports.SourceSearcher, catalog ports.TrackSourceWriter) *DiscoveryService {
	return &DiscoveryService{
		provider: provider, catalog: catalog, results: make(map[string]domain.TrackSource),
	}
}

func (s *DiscoveryService) Name() string { return s.provider.Name() }

func (s *DiscoveryService) Search(
	ctx context.Context, query domain.SearchQuery,
) ([]domain.TrackSource, error) {
	results, err := s.provider.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	for id, result := range s.results {
		if result.DiscoveredAt.IsZero() || now.Sub(result.DiscoveredAt) > discoveryRetention {
			delete(s.results, id)
		}
	}
	for _, result := range results {
		s.results[result.Track.ID] = result
	}
	s.mu.Unlock()
	return results, nil
}

func (s *DiscoveryService) Add(ctx context.Context, id string) (domain.Track, error) {
	s.mu.Lock()
	result, ok := s.results[id]
	s.mu.Unlock()
	if !ok {
		return domain.Track{}, ErrDiscoveryResultNotFound
	}
	if err := s.catalog.SaveTrackSource(ctx, result); err != nil {
		return domain.Track{}, fmt.Errorf("save discovered track: %w", err)
	}
	return result.Track, nil
}
