package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func TestPlayQueuePersistenceAndTemporaryTrackAbsence(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.db")
	catalog, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	track := domain.Track{
		ID: "track-1", Title: "Song", Artist: "Artist", ArtistID: "artist-1",
		Album: "Album", AlbumID: "album-1", AlbumArtist: "Artist",
	}
	source := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 17, 0, 0, 123, time.UTC)
	want := domain.PlayQueue{
		Owner: "alice", CurrentID: track.ID, PositionMS: 42000,
		Changed: now, ChangedBy: "mobile", Tracks: []domain.Track{track, track},
	}
	if err := catalog.SavePlayQueue(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", nil, nil); err != nil {
		t.Fatal(err)
	}
	unavailable, err := catalog.PlayQueue(ctx, "alice")
	if err != nil || len(unavailable.Tracks) != 0 || unavailable.CurrentID != track.ID {
		t.Fatalf("unavailable PlayQueue() = %#v, %v", unavailable, err)
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
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

	got, err := catalog.PlayQueue(ctx, "alice")
	if err != nil || got.CurrentID != want.CurrentID || got.PositionMS != want.PositionMS ||
		got.Changed != now || got.ChangedBy != want.ChangedBy || len(got.Tracks) != 2 {
		t.Fatalf("PlayQueue() = %#v, %v", got, err)
	}
}
