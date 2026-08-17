package services

import (
	"context"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type playlistCatalogStub struct {
	tracks    map[string]domain.Track
	playlists map[string]domain.Playlist
}

func (s *playlistCatalogStub) Track(_ context.Context, id string) (domain.Track, error) {
	track, ok := s.tracks[id]
	if !ok {
		return domain.Track{}, ports.ErrNotFound
	}
	return track, nil
}

func (s *playlistCatalogStub) Playlists(_ context.Context, owner string) ([]domain.Playlist, error) {
	var result []domain.Playlist
	for _, playlist := range s.playlists {
		if playlist.Owner == owner || playlist.Public {
			result = append(result, clonePlaylist(playlist))
		}
	}
	return result, nil
}

func (s *playlistCatalogStub) Playlist(_ context.Context, id string) (domain.Playlist, error) {
	playlist, ok := s.playlists[id]
	if !ok {
		return domain.Playlist{}, ports.ErrNotFound
	}
	return clonePlaylist(playlist), nil
}

func (s *playlistCatalogStub) SavePlaylist(_ context.Context, playlist domain.Playlist) error {
	s.playlists[playlist.ID] = clonePlaylist(playlist)
	return nil
}

func (s *playlistCatalogStub) DeletePlaylist(_ context.Context, id string) error {
	if _, ok := s.playlists[id]; !ok {
		return ports.ErrNotFound
	}
	delete(s.playlists, id)
	return nil
}

func clonePlaylist(playlist domain.Playlist) domain.Playlist {
	playlist.Tracks = append([]domain.Track(nil), playlist.Tracks...)
	return playlist
}

func TestPlaylistServiceCreatesUpdatesAndReplacesOrderedTracks(t *testing.T) {
	t.Parallel()

	firstTime := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(time.Hour)
	times := []time.Time{firstTime, secondTime, secondTime.Add(time.Hour)}
	catalog := &playlistCatalogStub{
		tracks: map[string]domain.Track{
			"one": {ID: "one", Duration: time.Minute},
			"two": {ID: "two", Duration: 2 * time.Minute},
		},
		playlists: make(map[string]domain.Playlist),
	}
	service := NewPlaylistService(catalog)
	service.newID = func() (string, error) { return "playlist_test", nil }
	service.now = func() time.Time {
		result := times[0]
		times = times[1:]
		return result
	}

	created, err := service.CreateOrReplace(context.Background(), "alice", "", "Roadtrip", []string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "playlist_test" || created.SongCount != 2 || created.Duration != 3*time.Minute {
		t.Fatalf("created playlist = %#v", created)
	}
	public := true
	name := "Renamed"
	updated, err := service.Update(context.Background(), "alice", created.ID, PlaylistUpdate{
		Name: &name, Public: &public, SongIndexesToRemove: []int{0}, SongIDsToAdd: []string{"one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || !updated.Public || updated.Created != firstTime || updated.Changed != secondTime ||
		len(updated.Tracks) != 2 || updated.Tracks[0].ID != "two" || updated.Tracks[1].ID != "one" {
		t.Fatalf("updated playlist = %#v", updated)
	}
	replaced, err := service.CreateOrReplace(context.Background(), "alice", created.ID, "", []string{"one", "one"})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Name != name || len(replaced.Tracks) != 2 || replaced.Tracks[0].ID != "one" || replaced.Tracks[1].ID != "one" {
		t.Fatalf("replaced playlist = %#v", replaced)
	}
}

func TestPlaylistServiceRejectsInvalidMutations(t *testing.T) {
	t.Parallel()

	catalog := &playlistCatalogStub{
		tracks: map[string]domain.Track{"one": {ID: "one"}},
		playlists: map[string]domain.Playlist{"playlist_test": {
			ID: "playlist_test", Name: "List", Owner: "alice", Tracks: []domain.Track{{ID: "one"}},
		}},
	}
	service := NewPlaylistService(catalog)
	for _, update := range []PlaylistUpdate{
		{SongIndexesToRemove: []int{1}},
		{SongIDsToAdd: []string{"missing"}},
	} {
		if _, err := service.Update(context.Background(), "alice", "playlist_test", update); err == nil {
			t.Fatalf("Update(%#v) error = nil", update)
		}
	}
	if _, err := service.Update(context.Background(), "bob", "playlist_test", PlaylistUpdate{}); err != ports.ErrNotFound {
		t.Fatalf("other owner update error = %v", err)
	}
}
