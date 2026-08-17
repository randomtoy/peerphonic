package services

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type StreamingService struct {
	catalog   ports.Catalog
	providers map[string]ports.SourceProvider
	order     map[string]int
}

func NewStreamingService(catalog ports.Catalog, providers ...ports.SourceProvider) *StreamingService {
	registry := make(map[string]ports.SourceProvider, len(providers))
	order := make(map[string]int, len(providers))
	for _, provider := range providers {
		if _, exists := order[provider.Name()]; !exists {
			order[provider.Name()] = len(order)
		}
		registry[provider.Name()] = provider
	}
	return &StreamingService{catalog: catalog, providers: registry, order: order}
}

func (s *StreamingService) Open(ctx context.Context, trackID string) (ports.ResolvedSource, error) {
	sources, err := s.catalog.Sources(ctx, trackID)
	if err != nil {
		return ports.ResolvedSource{}, fmt.Errorf("find sources for track %q: %w", trackID, err)
	}
	sort.SliceStable(sources, func(i, j int) bool {
		return s.providerOrder(sources[i].Provider) < s.providerOrder(sources[j].Provider)
	})

	var resolveErrors []error
	for _, source := range sources {
		provider, ok := s.providers[source.Provider]
		if !ok {
			resolveErrors = append(resolveErrors,
				fmt.Errorf("source provider %q is not registered", source.Provider))
			continue
		}
		stream, err := provider.Resolve(ctx, source)
		if err == nil {
			return stream, nil
		}
		if ctx.Err() != nil {
			return ports.ResolvedSource{}, fmt.Errorf("resolve track %q: %w", trackID, ctx.Err())
		}
		resolveErrors = append(resolveErrors, fmt.Errorf("resolve %s source: %w", source.Provider, err))
	}
	return ports.ResolvedSource{}, fmt.Errorf("resolve track %q from all sources: %w",
		trackID, errors.Join(resolveErrors...))
}

func (s *StreamingService) providerOrder(provider string) int {
	if order, ok := s.order[provider]; ok {
		return order
	}
	return len(s.order)
}
