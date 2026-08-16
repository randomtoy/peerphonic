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

func TestCatalogRoundTripAndReplacement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer catalog.Close()

	track := domain.Track{
		ID: "track-1", Title: "One", Artist: "Artist", ArtistID: "artist-1",
		Album: "Album", AlbumID: "album-1", AlbumArtist: "Artist",
		Source:      domain.SourceRef{Provider: "local", Key: "Artist/Album/One.flac"},
		TrackNumber: 1, Year: 2026, Duration: 3*time.Minute + 5*time.Second,
		Size: 42, BitRate: 900, Suffix: "flac", ContentType: "audio/flac",
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.Track{track}); err != nil {
		t.Fatalf("ReplaceProviderTracks() error = %v", err)
	}

	got, err := catalog.Track(ctx, track.ID)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if got.Title != track.Title || got.Source != track.Source || got.Duration != track.Duration {
		t.Fatalf("Track() = %#v, want %#v", got, track)
	}
	artists, err := catalog.Artists(ctx)
	if err != nil || len(artists) != 1 || artists[0].Name != "Artist" {
		t.Fatalf("Artists() = %#v, %v", artists, err)
	}
	if artists[0].AlbumCount != 1 {
		t.Fatalf("artist album count = %d, want 1", artists[0].AlbumCount)
	}
	allAlbums, err := catalog.Albums(ctx, 0, 10)
	if err != nil || len(allAlbums) != 1 || allAlbums[0].ID != "album-1" {
		t.Fatalf("Albums() = %#v, %v", allAlbums, err)
	}
	albums, err := catalog.AlbumsByArtist(ctx, "artist-1")
	if err != nil || len(albums) != 1 || albums[0].SongCount != 1 {
		t.Fatalf("AlbumsByArtist() = %#v, %v", albums, err)
	}
	tracks, err := catalog.TracksByAlbum(ctx, "album-1")
	if err != nil || len(tracks) != 1 || tracks[0].ID != track.ID {
		t.Fatalf("TracksByAlbum() = %#v, %v", tracks, err)
	}

	if err := catalog.ReplaceProviderTracks(ctx, "local", nil); err != nil {
		t.Fatalf("empty ReplaceProviderTracks() error = %v", err)
	}
	_, err = catalog.Track(ctx, track.ID)
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Track() after replacement error = %v, want ErrNotFound", err)
	}
}

func TestCatalogReplacementIsAtomic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	original := domain.Track{
		ID: "original", Title: "Original", Artist: "Artist", ArtistID: "artist",
		Album: "Album", AlbumID: "album", AlbumArtist: "Artist",
		Source: domain.SourceRef{Provider: "local", Key: "original.mp3"},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.Track{original}); err != nil {
		t.Fatal(err)
	}
	invalid := original
	invalid.ID = "invalid"
	invalid.Source.Provider = "remote"
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.Track{invalid}); err == nil {
		t.Fatal("ReplaceProviderTracks() error = nil, want provider mismatch")
	}
	if _, err := catalog.Track(ctx, original.ID); err != nil {
		t.Fatalf("original track lost after rollback: %v", err)
	}
}
