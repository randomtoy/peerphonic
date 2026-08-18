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
var ErrCollectionBrowsingUnsupported = errors.New("source collection browsing is not supported")

const discoveryRetention = 30 * time.Minute

type DiscoveryService struct {
	provider ports.SourceSearcher
	catalog  ports.TrackSourceWriter

	mu          sync.Mutex
	results     map[string]domain.TrackSource
	collections map[string]cachedCollection
}

type cachedCollection struct {
	value    domain.SourceCollection
	cachedAt time.Time
}

func NewDiscoveryService(provider ports.SourceSearcher, catalog ports.TrackSourceWriter) *DiscoveryService {
	return &DiscoveryService{
		provider: provider, catalog: catalog, results: make(map[string]domain.TrackSource),
		collections: make(map[string]cachedCollection),
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
			delete(s.collections, id)
		}
	}
	for _, result := range results {
		s.results[result.Track.ID] = result
		delete(s.collections, result.Track.ID)
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

func (s *DiscoveryService) AddCollection(ctx context.Context, id string) (domain.SourceCollection, error) {
	collection, err := s.PreviewCollection(ctx, id)
	if err != nil {
		return domain.SourceCollection{}, err
	}
	if err := s.catalog.SaveTrackSources(ctx, collection.Tracks); err != nil {
		return domain.SourceCollection{}, fmt.Errorf("save discovered collection: %w", err)
	}
	return collection, nil
}

func (s *DiscoveryService) PreviewCollection(ctx context.Context, id string) (domain.SourceCollection, error) {
	now := time.Now().UTC()
	s.mu.Lock()
	result, ok := s.results[id]
	cached, cachedOK := s.collections[id]
	s.mu.Unlock()
	if !ok {
		return domain.SourceCollection{}, ErrDiscoveryResultNotFound
	}
	if cachedOK && now.Sub(cached.cachedAt) <= discoveryRetention {
		return cached.value, nil
	}
	browser, ok := s.provider.(ports.SourceCollectionBrowser)
	if !ok {
		return domain.SourceCollection{}, ErrCollectionBrowsingUnsupported
	}
	collection, err := browser.BrowseCollection(ctx, result)
	if err != nil {
		return domain.SourceCollection{}, fmt.Errorf("browse discovered collection: %w", err)
	}
	if len(collection.Tracks) == 0 {
		return domain.SourceCollection{}, errors.New("discovered collection has no tracks")
	}
	s.mu.Lock()
	s.collections[id] = cachedCollection{value: collection, cachedAt: now}
	s.mu.Unlock()
	return collection, nil
}
