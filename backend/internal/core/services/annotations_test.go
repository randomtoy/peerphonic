package services

import (
	"context"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type annotationMemory struct {
	tracks      map[string]domain.Track
	artists     map[string]domain.Artist
	annotations map[string]map[domain.MediaRef]domain.MediaAnnotation
}

func (m *annotationMemory) Track(_ context.Context, id string) (domain.Track, error) {
	track, ok := m.tracks[id]
	if !ok {
		return domain.Track{}, ports.ErrNotFound
	}
	return track, nil
}

func (m *annotationMemory) TracksByAlbum(_ context.Context, id string) ([]domain.Track, error) {
	var tracks []domain.Track
	for _, track := range m.tracks {
		if track.AlbumID == id {
			tracks = append(tracks, track)
		}
	}
	return tracks, nil
}

func (m *annotationMemory) Artist(_ context.Context, id string) (domain.Artist, error) {
	artist, ok := m.artists[id]
	if !ok {
		return domain.Artist{}, ports.ErrNotFound
	}
	return artist, nil
}

func (m *annotationMemory) MediaAnnotations(_ context.Context, owner string) ([]domain.MediaAnnotation, error) {
	var result []domain.MediaAnnotation
	for _, annotation := range m.annotations[owner] {
		result = append(result, annotation)
	}
	return result, nil
}

func (m *annotationMemory) UpdateMediaAnnotations(_ context.Context, updates []ports.MediaAnnotationUpdate) error {
	for _, update := range updates {
		if m.annotations[update.Owner] == nil {
			m.annotations[update.Owner] = make(map[domain.MediaRef]domain.MediaAnnotation)
		}
		item := m.annotations[update.Owner][update.Media]
		item.Owner, item.Media = update.Owner, update.Media
		if update.StarredAt != nil {
			item.StarredAt = *update.StarredAt
		}
		if update.Rating != nil {
			item.Rating = *update.Rating
		}
		item.PlayCount += update.PlayCountDelta
		if update.LastPlayed != nil {
			item.LastPlayed = *update.LastPlayed
		}
		m.annotations[update.Owner][update.Media] = item
	}
	return nil
}

func TestAnnotationLifecycleAndScrobble(t *testing.T) {
	t.Parallel()

	track := domain.Track{
		ID: "track-1", Title: "Song", Artist: "Artist", ArtistID: "artist-1",
		Album: "Album", AlbumID: "album-1", AlbumArtist: "Artist", AlbumArtistID: "artist-1",
		Duration: 3 * time.Minute,
	}
	memory := &annotationMemory{
		tracks: map[string]domain.Track{track.ID: track},
		artists: map[string]domain.Artist{
			"artist-1": {ID: "artist-1", Name: "Artist", AlbumCount: 1},
		},
		annotations: make(map[string]map[domain.MediaRef]domain.MediaAnnotation),
	}
	service := NewAnnotationService(memory, memory)
	now := time.Date(2026, 8, 17, 13, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	ctx := context.Background()

	if err := service.SetStarred(ctx, "alice", []domain.MediaRef{
		{ID: track.ID},
		{Type: domain.MediaAlbum, ID: track.AlbumID},
		{Type: domain.MediaArtist, ID: track.ArtistID},
	}, true); err != nil {
		t.Fatal(err)
	}
	if err := service.SetRating(ctx, "alice", domain.MediaRef{ID: track.ID}, 4); err != nil {
		t.Fatal(err)
	}
	playedAt := now.Add(-time.Minute)
	if err := service.Scrobble(ctx, "alice", []string{track.ID, track.ID},
		[]time.Time{playedAt, now}, true); err != nil {
		t.Fatal(err)
	}
	if err := service.Scrobble(ctx, "alice", []string{track.ID}, nil, false); err != nil {
		t.Fatal(err)
	}

	states, err := service.All(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	song := states[domain.MediaRef{Type: domain.MediaSong, ID: track.ID}]
	if song.StarredAt != now || song.Rating != 4 || song.PlayCount != 2 || song.LastPlayed != now {
		t.Fatalf("song annotation = %#v", song)
	}
	starred, err := service.Starred(ctx, "alice")
	if err != nil || len(starred.Tracks) != 1 || len(starred.Albums) != 1 || len(starred.Artists) != 1 {
		t.Fatalf("Starred() = %#v, %v", starred, err)
	}
	if err := service.SetStarred(ctx, "alice", []domain.MediaRef{{ID: track.ID}}, false); err != nil {
		t.Fatal(err)
	}
	states, err = service.All(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	song = states[domain.MediaRef{Type: domain.MediaSong, ID: track.ID}]
	if !song.StarredAt.IsZero() || song.Rating != 4 || song.PlayCount != 2 {
		t.Fatalf("annotation after unstar = %#v", song)
	}
}

func TestAnnotationValidation(t *testing.T) {
	t.Parallel()

	memory := &annotationMemory{
		tracks: map[string]domain.Track{}, artists: map[string]domain.Artist{},
		annotations: make(map[string]map[domain.MediaRef]domain.MediaAnnotation),
	}
	service := NewAnnotationService(memory, memory)
	if err := service.SetRating(context.Background(), "alice", domain.MediaRef{ID: "missing"}, 6); err == nil {
		t.Fatal("SetRating() accepted an invalid rating")
	}
	if err := service.Scrobble(context.Background(), "alice", []string{"missing"}, nil, true); err == nil {
		t.Fatal("Scrobble() accepted a missing song")
	}
}
