package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestPlaylistPersistenceAndOrderedTracks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.db")
	catalog, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	track := domain.Track{
		ID: "track-1", Title: "Song", Artist: "Artist", ArtistID: "artist-1",
		Album: "Album", AlbumID: "album-1", AlbumArtist: "Artist", Duration: 90 * time.Second,
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{{
		Track: track, Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 12, 0, 0, 123, time.UTC)
	want := domain.Playlist{
		ID: "playlist-1", Name: "Saved", Comment: "Offline", Owner: "alice", Public: true,
		Created: now, Changed: now, Tracks: []domain.Track{track, track},
	}
	if err := catalog.SavePlaylist(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", nil, nil); err != nil {
		t.Fatal(err)
	}
	unavailable, err := catalog.Playlist(ctx, want.ID)
	if err != nil || len(unavailable.Tracks) != 0 {
		t.Fatalf("playlist with unavailable source = %#v, %v", unavailable, err)
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{{
		Track: track, Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Close(); err != nil {
		t.Fatal(err)
	}
	catalog, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	items, err := catalog.Playlists(ctx, "alice")
	if err != nil || len(items) != 1 || items[0].SongCount != 2 || items[0].Duration != 3*time.Minute {
		t.Fatalf("Playlists() = %#v, %v", items, err)
	}
	got, err := catalog.Playlist(ctx, want.ID)
	if err != nil || len(got.Tracks) != 2 || got.Tracks[0].ID != track.ID || got.Tracks[1].ID != track.ID ||
		got.Created != now || got.Changed != now {
		t.Fatalf("Playlist() = %#v, %v", got, err)
	}
	if err := catalog.DeletePlaylist(ctx, want.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Playlist(ctx, want.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Playlist() after delete error = %v", err)
	}
}
