package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const maxPlayQueueTracks = 10000

var ErrInvalidPlayQueue = errors.New("invalid play queue")

type playQueueCatalog interface {
	Track(ctx context.Context, id string) (domain.Track, error)
}

type PlayQueueService struct {
	catalog playQueueCatalog
	store   ports.PlayQueueStore
	now     func() time.Time
}

func NewPlayQueueService(catalog playQueueCatalog, store ports.PlayQueueStore) *PlayQueueService {
	return &PlayQueueService{catalog: catalog, store: store, now: time.Now}
}

func (s *PlayQueueService) Get(ctx context.Context, owner string) (domain.PlayQueue, error) {
	queue, err := s.store.PlayQueue(ctx, owner)
	if errors.Is(err, ports.ErrNotFound) {
		return domain.PlayQueue{
			Owner: owner, Changed: time.Unix(0, 0).UTC(), Tracks: []domain.Track{},
		}, nil
	}
	if err != nil {
		return domain.PlayQueue{}, err
	}
	if len(queue.Tracks) == 0 {
		queue.CurrentID = ""
		queue.PositionMS = 0
		return queue, nil
	}
	for _, track := range queue.Tracks {
		if track.ID == queue.CurrentID {
			return queue, nil
		}
	}
	queue.CurrentID = queue.Tracks[0].ID
	queue.PositionMS = 0
	return queue, nil
}

func (s *PlayQueueService) Save(
	ctx context.Context,
	owner string,
	trackIDs []string,
	currentID string,
	positionMS int64,
	changedBy string,
) (domain.PlayQueue, error) {
	if positionMS < 0 {
		return domain.PlayQueue{}, fmt.Errorf("position must be non-negative: %w", ErrInvalidPlayQueue)
	}
	if len(trackIDs) > maxPlayQueueTracks {
		return domain.PlayQueue{}, fmt.Errorf("play queue cannot contain more than %d tracks: %w",
			maxPlayQueueTracks, ErrInvalidPlayQueue)
	}
	currentID = strings.TrimSpace(currentID)
	if len(trackIDs) != 0 && currentID == "" {
		return domain.PlayQueue{}, fmt.Errorf("current song is required for a non-empty queue: %w", ErrInvalidPlayQueue)
	}
	if len(trackIDs) == 0 && currentID != "" {
		return domain.PlayQueue{}, fmt.Errorf("current song must be empty when clearing the queue: %w", ErrInvalidPlayQueue)
	}

	tracks := make([]domain.Track, 0, len(trackIDs))
	currentFound := len(trackIDs) == 0
	for _, id := range trackIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return domain.PlayQueue{}, fmt.Errorf("song id is empty: %w", ErrInvalidPlayQueue)
		}
		track, err := s.catalog.Track(ctx, id)
		if err != nil {
			return domain.PlayQueue{}, fmt.Errorf("resolve play queue song %q: %w", id, err)
		}
		tracks = append(tracks, track)
		if id == currentID {
			currentFound = true
		}
	}
	if !currentFound {
		return domain.PlayQueue{}, fmt.Errorf("current song is not in the queue: %w", ErrInvalidPlayQueue)
	}
	changedBy = strings.TrimSpace(changedBy)
	if changedBy == "" {
		changedBy = "unknown"
	}
	queue := domain.PlayQueue{
		Owner: owner, CurrentID: currentID, PositionMS: positionMS,
		Changed: s.now().UTC(), ChangedBy: changedBy, Tracks: tracks,
	}
	if len(tracks) == 0 {
		queue.PositionMS = 0
	}
	if err := s.store.SavePlayQueue(ctx, queue); err != nil {
		return domain.PlayQueue{}, err
	}
	return queue, nil
}
