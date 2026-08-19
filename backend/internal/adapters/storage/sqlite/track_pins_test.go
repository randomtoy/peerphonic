package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func TestTrackPinsResolveLegacyAliases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	track := domain.Track{ID: "logical", Title: "Song", Artist: "Artist", ArtistID: "artist",
		Album: "Album", AlbumID: "album", AlbumArtist: "Artist", AlbumArtistID: "artist",
		Suffix: "mp3", ContentType: "audio/mpeg"}
	if err := catalog.SaveTrackSource(ctx, domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveTrackAlias(ctx, "legacy", track.ID); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SetTrackPinned(ctx, "legacy", true); err != nil {
		t.Fatal(err)
	}
	if pinned, err := catalog.TrackPinned(ctx, track.ID); err != nil || !pinned {
		t.Fatalf("TrackPinned() = %v, %v", pinned, err)
	}
	if err := catalog.SetTrackPinned(ctx, track.ID, false); err != nil {
		t.Fatal(err)
	}
	if pinned, err := catalog.TrackPinned(ctx, track.ID); err != nil || pinned {
		t.Fatalf("TrackPinned() after unpin = %v, %v", pinned, err)
	}
}
