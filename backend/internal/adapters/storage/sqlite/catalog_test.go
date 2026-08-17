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
		TrackNumber: 1, Year: 2026, Duration: 3*time.Minute + 5*time.Second,
		Size: 42, BitRate: 900, Suffix: "flac", ContentType: "audio/flac", CoverArtID: "art-1",
	}
	source := domain.SourceRef{Provider: "local", Key: "Artist/Album/One.flac"}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{{Track: track, Ref: source}}, nil); err != nil {
		t.Fatalf("ReplaceProviderTracks() error = %v", err)
	}

	got, err := catalog.Track(ctx, track.ID)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if got.Title != track.Title || got.Duration != track.Duration ||
		got.CoverArtID != track.CoverArtID {
		t.Fatalf("Track() = %#v, want %#v", got, track)
	}
	sources, err := catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 1 || sources[0] != source {
		t.Fatalf("Sources() = %#v, %v", sources, err)
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
	if allAlbums[0].CoverArtID != "art-1" {
		t.Fatalf("album cover art ID = %q", allAlbums[0].CoverArtID)
	}
	albums, err := catalog.AlbumsByArtist(ctx, "artist-1")
	if err != nil || len(albums) != 1 || albums[0].SongCount != 1 {
		t.Fatalf("AlbumsByArtist() = %#v, %v", albums, err)
	}
	tracks, err := catalog.TracksByAlbum(ctx, "album-1")
	if err != nil || len(tracks) != 1 || tracks[0].ID != track.ID {
		t.Fatalf("TracksByAlbum() = %#v, %v", tracks, err)
	}

	if err := catalog.ReplaceProviderTracks(ctx, "local", nil, nil); err != nil {
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
	}
	originalSource := domain.TrackSource{
		Track: original, Ref: domain.SourceRef{Provider: "local", Key: "original.mp3"},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{originalSource}, nil); err != nil {
		t.Fatal(err)
	}
	invalid := originalSource
	invalid.Track.ID = "invalid"
	invalid.Ref.Provider = "remote"
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{invalid}, nil); err == nil {
		t.Fatal("ReplaceProviderTracks() error = nil, want provider mismatch")
	}
	if _, err := catalog.Track(ctx, original.ID); err != nil {
		t.Fatalf("original track lost after rollback: %v", err)
	}
}

func TestCatalogRetainsAlbumAliasesAcrossMetadataChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	source := domain.TrackSource{
		Track: domain.Track{
			ID: "track-1", Title: "Song", Artist: "Artist", ArtistID: "artist",
			Album: "First", AlbumID: "album-first", AlbumArtist: "Artist",
		},
		Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
		t.Fatal(err)
	}
	source.Track.Album = "Second"
	source.Track.AlbumID = "album-second"
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
		t.Fatal(err)
	}
	source.Track.Album = "Third"
	source.Track.AlbumID = "album-third"
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
		t.Fatal(err)
	}
	for _, albumID := range []string{"album-first", "album-second", "album-third"} {
		tracks, err := catalog.TracksByAlbum(ctx, albumID)
		if err != nil || len(tracks) != 1 || tracks[0].AlbumID != "album-third" {
			t.Fatalf("TracksByAlbum(%q) = %#v, %v", albumID, tracks, err)
		}
	}
}

func TestCatalogKeepsTrackUntilItsLastSourceIsRemoved(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	track := domain.Track{
		ID: "shared-track", Title: "Shared", Artist: "Artist", ArtistID: "artist",
		Album: "Album", AlbumID: "album", AlbumArtist: "Artist",
	}
	localSource := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "local", Key: "shared.mp3"},
	}
	remoteSource := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "remote", Key: "peer/shared.mp3"},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{localSource}, nil); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ReplaceProviderTracks(ctx, "remote", []domain.TrackSource{remoteSource}, nil); err != nil {
		t.Fatal(err)
	}
	sources, err := catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 2 {
		t.Fatalf("Sources() = %#v, %v; want two sources", sources, err)
	}

	if err := catalog.ReplaceProviderTracks(ctx, "local", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Track(ctx, track.ID); err != nil {
		t.Fatalf("Track() after local removal error = %v", err)
	}
	sources, err = catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 1 || sources[0] != remoteSource.Ref {
		t.Fatalf("Sources() after local removal = %#v, %v", sources, err)
	}

	if err := catalog.ReplaceProviderTracks(ctx, "remote", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Track(ctx, track.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Track() after final removal error = %v, want ErrNotFound", err)
	}
}

func TestCatalogSearchIsUnicodeAwareAndPagedIndependently(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	tracks := []domain.TrackSource{
		{
			Track: domain.Track{
				ID: "track-crow", Title: "Пластилиновая Ворона", Artist: "Черная Метка",
				ArtistID: "artist-black", Album: "Hard Covers", AlbumID: "album-hard",
				AlbumArtist: "Черная Метка",
			},
			Ref: domain.SourceRef{Provider: "local", Key: "crow.mp3"},
		},
		{
			Track: domain.Track{
				ID: "track-song", Title: "Another Song", Artist: "Other Artist",
				ArtistID: "artist-other", Album: "Other Album", AlbumID: "album-other",
				AlbumArtist: "Other Artist",
			},
			Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
		},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", tracks, nil); err != nil {
		t.Fatal(err)
	}

	result, err := catalog.Search(ctx, ports.CatalogSearch{
		Text: "черная", ArtistCount: 10, AlbumCount: 10, SongCount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artists) != 1 || result.Artists[0].ID != "artist-black" {
		t.Fatalf("artists = %#v", result.Artists)
	}
	if len(result.Albums) != 1 || result.Albums[0].ID != "album-hard" {
		t.Fatalf("albums = %#v", result.Albums)
	}
	if len(result.Songs) != 1 || result.Songs[0].ID != "track-crow" {
		t.Fatalf("songs = %#v", result.Songs)
	}

	result, err = catalog.Search(ctx, ports.CatalogSearch{
		Text: "", ArtistOffset: 1, ArtistCount: 1, AlbumCount: 0, SongOffset: 1, SongCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artists) != 1 || len(result.Albums) != 0 || len(result.Songs) != 1 {
		t.Fatalf("paged result = %#v", result)
	}
}
