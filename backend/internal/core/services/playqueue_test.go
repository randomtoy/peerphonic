package services

import (
	"context"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type playQueueMemory struct {
	tracks map[string]domain.Track
	queue  *domain.PlayQueue
}

func (m *playQueueMemory) Track(_ context.Context, id string) (domain.Track, error) {
	track, ok := m.tracks[id]
	if !ok {
		return domain.Track{}, ports.ErrNotFound
	}
	return track, nil
}

func (m *playQueueMemory) PlayQueue(_ context.Context, _ string) (domain.PlayQueue, error) {
	if m.queue == nil {
		return domain.PlayQueue{}, ports.ErrNotFound
	}
	return *m.queue, nil
}

func (m *playQueueMemory) SavePlayQueue(_ context.Context, queue domain.PlayQueue) error {
	m.queue = &queue
	return nil
}

func TestPlayQueueSaveGetAndClear(t *testing.T) {
	t.Parallel()

	first := domain.Track{ID: "track-1", Title: "One"}
	second := domain.Track{ID: "track-2", Title: "Two"}
	memory := &playQueueMemory{tracks: map[string]domain.Track{
		first.ID: first, second.ID: second,
	}}
	service := NewPlayQueueService(memory, memory)
	now := time.Date(2026, 8, 17, 17, 30, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	ctx := context.Background()

	empty, err := service.Get(ctx, "alice")
	if err != nil || empty.Owner != "alice" || len(empty.Tracks) != 0 {
		t.Fatalf("empty Get() = %#v, %v", empty, err)
	}
	saved, err := service.Save(ctx, "alice",
		[]string{first.ID, second.ID, first.ID}, second.ID, 12000, "mobile")
	if err != nil {
		t.Fatal(err)
	}
	if saved.CurrentID != second.ID || saved.PositionMS != 12000 || saved.Changed != now ||
		saved.ChangedBy != "mobile" || len(saved.Tracks) != 3 {
		t.Fatalf("Save() = %#v", saved)
	}
	got, err := service.Get(ctx, "alice")
	if err != nil || len(got.Tracks) != 3 || got.Tracks[2].ID != first.ID {
		t.Fatalf("Get() = %#v, %v", got, err)
	}
	cleared, err := service.Save(ctx, "alice", nil, "", 999, "mobile")
	if err != nil || len(cleared.Tracks) != 0 || cleared.PositionMS != 0 || cleared.CurrentID != "" {
		t.Fatalf("cleared Save() = %#v, %v", cleared, err)
	}
}

func TestPlayQueueValidation(t *testing.T) {
	t.Parallel()

	track := domain.Track{ID: "track-1"}
	memory := &playQueueMemory{tracks: map[string]domain.Track{track.ID: track}}
	service := NewPlayQueueService(memory, memory)
	tests := []struct {
		name     string
		ids      []string
		current  string
		position int64
	}{
		{name: "missing current", ids: []string{track.ID}},
		{name: "current outside queue", ids: []string{track.ID}, current: "track-2"},
		{name: "current when clearing", current: track.ID},
		{name: "negative position", position: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Save(context.Background(), "alice",
				test.ids, test.current, test.position, "test"); err == nil {
				t.Fatal("Save() accepted an invalid queue")
			}
		})
	}
}
