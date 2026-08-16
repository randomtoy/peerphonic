package services

import (
	"context"
	"fmt"

	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type StreamingService struct {
	catalog   ports.Catalog
	providers map[string]ports.SourceProvider
}

func NewStreamingService(catalog ports.Catalog, providers ...ports.SourceProvider) *StreamingService {
	registry := make(map[string]ports.SourceProvider, len(providers))
	for _, provider := range providers {
		registry[provider.Name()] = provider
	}
	return &StreamingService{catalog: catalog, providers: registry}
}

func (s *StreamingService) Open(ctx context.Context, trackID string) (ports.ResolvedSource, error) {
	track, err := s.catalog.Track(ctx, trackID)
	if err != nil {
		return ports.ResolvedSource{}, fmt.Errorf("find track %q: %w", trackID, err)
	}
	provider, ok := s.providers[track.Source.Provider]
	if !ok {
		return ports.ResolvedSource{}, fmt.Errorf("source provider %q is not registered", track.Source.Provider)
	}
	stream, err := provider.Resolve(ctx, track.Source)
	if err != nil {
		return ports.ResolvedSource{}, fmt.Errorf("resolve track %q: %w", trackID, err)
	}
	return stream, nil
}
