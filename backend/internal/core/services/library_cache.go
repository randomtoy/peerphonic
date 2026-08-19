package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type LibraryCacheService struct {
	ctx     context.Context
	catalog ports.Catalog
	streams *StreamingService
	pins    ports.TrackPinStore
	sources ports.SourceManager

	mu      sync.Mutex
	active  map[string]struct{}
	workers chan struct{}
}

func NewLibraryCacheService(
	ctx context.Context, catalog ports.Catalog, streams *StreamingService,
	pins ports.TrackPinStore, sources ports.SourceManager,
) *LibraryCacheService {
	return &LibraryCacheService{
		ctx: ctx, catalog: catalog, streams: streams, pins: pins, sources: sources,
		active: make(map[string]struct{}), workers: make(chan struct{}, 2),
	}
}

func (s *LibraryCacheService) PrefetchTrack(ctx context.Context, trackID string, pinned bool) error {
	track, err := s.catalog.Track(ctx, trackID)
	if err != nil {
		return err
	}
	if pinned {
		if err := s.SetTrackPinned(ctx, track.ID, true); err != nil {
			return err
		}
	}
	s.queue(track.ID)
	return nil
}

func (s *LibraryCacheService) PrefetchAlbum(
	ctx context.Context, albumID string, pinned bool,
) (int, error) {
	tracks, err := s.catalog.TracksByAlbum(ctx, albumID)
	if err != nil {
		return 0, err
	}
	if len(tracks) == 0 {
		return 0, ports.ErrNotFound
	}
	for _, track := range tracks {
		if pinned {
			if err := s.SetTrackPinned(ctx, track.ID, true); err != nil {
				return 0, err
			}
		}
		s.queue(track.ID)
	}
	return len(tracks), nil
}

func (s *LibraryCacheService) SetTrackPinned(ctx context.Context, trackID string, pinned bool) error {
	track, err := s.catalog.Track(ctx, trackID)
	if err != nil {
		return err
	}
	if err := s.pins.SetTrackPinned(ctx, track.ID, pinned); err != nil {
		return err
	}
	if s.sources == nil {
		return nil
	}
	sources, err := s.catalog.Sources(ctx, track.ID)
	if err != nil {
		return err
	}
	for _, source := range sources {
		if source.Provider != "torrent" {
			continue
		}
		infoHash, _, ok := strings.Cut(source.Key, "/")
		if !ok || infoHash == "" {
			continue
		}
		keepPinned := pinned
		if !pinned {
			keepPinned, err = s.torrentHasPinnedTrack(ctx, infoHash)
			if err != nil {
				return err
			}
		}
		if err := s.sources.PinSource(ctx, infoHash, keepPinned); err != nil && !errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("pin torrent source: %w", err)
		}
	}
	return nil
}

func (s *LibraryCacheService) torrentHasPinnedTrack(ctx context.Context, infoHash string) (bool, error) {
	trackIDs, err := s.pins.PinnedTracks(ctx)
	if err != nil {
		return false, err
	}
	for _, trackID := range trackIDs {
		sources, err := s.catalog.Sources(ctx, trackID)
		if err != nil {
			continue
		}
		for _, source := range sources {
			if source.Provider == "torrent" && strings.HasPrefix(source.Key, infoHash+"/") {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *LibraryCacheService) queue(trackID string) {
	s.mu.Lock()
	if _, exists := s.active[trackID]; exists {
		s.mu.Unlock()
		return
	}
	s.active[trackID] = struct{}{}
	s.mu.Unlock()
	go func() {
		select {
		case s.workers <- struct{}{}:
		case <-s.ctx.Done():
			s.complete(trackID)
			return
		}
		defer func() { <-s.workers; s.complete(trackID) }()
		resolved, err := s.streams.Open(s.ctx, trackID)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resolved.Content)
		_ = resolved.Content.Close()
	}()
}

func (s *LibraryCacheService) complete(trackID string) {
	s.mu.Lock()
	delete(s.active, trackID)
	s.mu.Unlock()
}
