package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

var ErrDiscoveryResultNotFound = errors.New("discovery result not found")
var ErrCollectionBrowsingUnsupported = errors.New("source collection browsing is not supported")

const (
	discoveryRetention        = 30 * time.Minute
	discoveryAlternativeLimit = 100
	discoveryAlternativeMax   = 8
)

var discoveryStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "of": {}, "the": {},
}

var discoveryVersionTokens = map[string]struct{}{
	"acoustic": {}, "cover": {}, "demo": {}, "instrumental": {}, "karaoke": {},
	"live": {}, "remix": {}, "remixed": {},
}

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
	alternatives := s.findAlternativeSources(ctx, result.Track)
	sources := make([]domain.TrackSource, 0, len(alternatives)+1)
	for _, alternative := range alternatives {
		if alternative.Ref != result.Ref {
			sources = append(sources, alternative)
		}
	}
	sources = append(sources, result)
	if err := s.catalog.SaveTrackSources(ctx, sources); err != nil {
		return domain.Track{}, fmt.Errorf("save discovered track: %w", err)
	}
	return result.Track, nil
}

func (s *DiscoveryService) Refresh(ctx context.Context, track domain.Track) error {
	sources := s.findAlternativeSources(ctx, track)
	if len(sources) == 0 {
		return ErrDiscoveryResultNotFound
	}
	if err := s.catalog.SaveTrackSources(ctx, sources); err != nil {
		return fmt.Errorf("save alternative track sources: %w", err)
	}
	return nil
}

func (s *DiscoveryService) findAlternativeSources(
	ctx context.Context, selected domain.Track,
) []domain.TrackSource {
	query := discoveryAlternativeQuery(selected)
	if query == "" {
		return nil
	}
	results, err := s.provider.Search(ctx, domain.SearchQuery{Text: query, Limit: discoveryAlternativeLimit})
	if err != nil {
		return nil
	}
	sort.SliceStable(results, func(i, j int) bool {
		return betterDiscoveryAlternative(results[i], results[j], selected.Suffix)
	})

	now := time.Now().UTC()
	sources := make([]domain.TrackSource, 0, discoveryAlternativeMax+1)
	seen := make(map[domain.SourceRef]struct{})
	for _, candidate := range results {
		if candidate.Availability.RequiresApproval || !sameDiscoveredRecording(selected, candidate) {
			continue
		}
		if _, exists := seen[candidate.Ref]; exists {
			continue
		}
		seen[candidate.Ref] = struct{}{}
		candidate.Track = selected
		candidate.DiscoveredAt = now.Add(-time.Duration(len(sources)) * time.Nanosecond)
		sources = append(sources, candidate)
		if len(sources) == discoveryAlternativeMax {
			break
		}
	}
	return sources
}

func discoveryAlternativeQuery(track domain.Track) string {
	artistTokens := meaningfulDiscoveryTokens(track.Artist, false)
	titleTokens := meaningfulDiscoveryTokens(track.Title, true)
	if len(titleTokens) == 0 {
		return ""
	}
	query := make([]string, 0, len(titleTokens)+1)
	if len(artistTokens) > 0 {
		longest := artistTokens[0]
		for _, token := range artistTokens[1:] {
			if len([]rune(token)) > len([]rune(longest)) {
				longest = token
			}
		}
		query = append(query, longest)
	}
	query = append(query, titleTokens...)
	return strings.Join(query, " ")
}

func sameDiscoveredRecording(selected domain.Track, candidate domain.TrackSource) bool {
	haystack := make(map[string]struct{})
	for _, token := range discoveryTokens(strings.Join([]string{
		candidate.Track.Artist, candidate.Track.Title, candidate.DisplayPath,
	}, " ")) {
		haystack[token] = struct{}{}
	}
	for _, token := range append(
		meaningfulDiscoveryTokens(selected.Artist, false),
		meaningfulDiscoveryTokens(selected.Title, true)...,
	) {
		if _, ok := haystack[token]; !ok {
			return false
		}
	}
	selectedVersions := make(map[string]struct{})
	for _, token := range discoveryTokens(selected.Title) {
		if _, version := discoveryVersionTokens[token]; version {
			selectedVersions[token] = struct{}{}
		}
	}
	for token := range discoveryVersionTokens {
		_, selectedHas := selectedVersions[token]
		_, candidateHas := haystack[token]
		if selectedHas != candidateHas {
			return false
		}
	}
	if selected.Duration > 0 && candidate.Track.Duration > 0 {
		difference := selected.Duration - candidate.Track.Duration
		if difference < 0 {
			difference = -difference
		}
		if difference > 20*time.Second {
			return false
		}
	}
	return true
}

func meaningfulDiscoveryTokens(value string, trimTrackNumber bool) []string {
	tokens := discoveryTokens(value)
	if trimTrackNumber {
		for len(tokens) > 0 && isNumericToken(tokens[0]) {
			tokens = tokens[1:]
		}
	}
	filtered := tokens[:0]
	for _, token := range tokens {
		if _, stop := discoveryStopWords[token]; !stop {
			filtered = append(filtered, token)
		}
	}
	return filtered
}

func discoveryTokens(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func isNumericToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsNumber(r) {
			return false
		}
	}
	return true
}

func betterDiscoveryAlternative(left, right domain.TrackSource, preferredSuffix string) bool {
	leftFormat := strings.EqualFold(left.Track.Suffix, preferredSuffix)
	rightFormat := strings.EqualFold(right.Track.Suffix, preferredSuffix)
	if leftFormat != rightFormat {
		return leftFormat
	}
	if left.Availability.FreeUploadSlot != right.Availability.FreeUploadSlot {
		return left.Availability.FreeUploadSlot
	}
	if left.Availability.QueueLength != right.Availability.QueueLength {
		return left.Availability.QueueLength < right.Availability.QueueLength
	}
	return left.Availability.UploadSpeed > right.Availability.UploadSpeed
}

func (s *DiscoveryService) Result(id string) (domain.TrackSource, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	result, ok := s.results[id]
	if !ok {
		return domain.TrackSource{}, false
	}
	if result.DiscoveredAt.IsZero() || now.Sub(result.DiscoveredAt) > discoveryRetention {
		delete(s.results, id)
		delete(s.collections, id)
		return domain.TrackSource{}, false
	}
	return result, true
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
