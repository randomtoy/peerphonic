package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type extractorStub struct{}

func (extractorStub) Extract(path string, info os.FileInfo) (Metadata, error) {
	return Metadata{
		Title: filepath.Base(path), Artist: "Track Artist", Album: "Album",
		AlbumArtist: "Album Artist", Size: info.Size(), Suffix: "mp3", ContentType: "audio/mpeg",
		Artwork: &Artwork{Data: []byte("image")},
	}, nil
}

type artworkWriterStub struct {
	writes int
}

func (s *artworkWriterStub) Put(context.Context, []byte) (string, error) {
	s.writes++
	return "art_test", nil
}

func TestScanBuildsAndReplacesLocalCatalog(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	first := filepath.Join(root, "first.mp3")
	second := filepath.Join(root, "nested", "second.FLAC")
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{first, second, filepath.Join(root, "ignored.txt")} {
		if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	artwork := &artworkWriterStub{}
	s := New(root, catalog, extractorStub{}, artwork)
	report, err := s.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if report.Tracks != 2 || len(report.Warnings) != 0 {
		t.Fatalf("Scan() report = %#v", report)
	}
	artists, err := catalog.Artists(ctx)
	if err != nil || len(artists) != 2 {
		t.Fatalf("Artists() = %#v, %v", artists, err)
	}
	albums, err := catalog.AlbumsByArtist(ctx, artists[0].ID)
	if err != nil || len(albums) != 1 {
		t.Fatalf("AlbumsByArtist() = %#v, %v", albums, err)
	}
	tracks, err := catalog.TracksByAlbum(ctx, albums[0].ID)
	if err != nil || len(tracks) != 2 {
		t.Fatalf("TracksByAlbum() = %#v, %v", tracks, err)
	}
	if tracks[0].CoverArtID != "art_test" || artwork.writes != 1 {
		t.Fatalf("cover art ID = %q, writes = %d", tracks[0].CoverArtID, artwork.writes)
	}

	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	tracks, err = catalog.TracksByAlbum(ctx, albums[0].ID)
	if err != nil || len(tracks) != 1 {
		t.Fatalf("tracks after rescan = %#v, %v", tracks, err)
	}
}

type compilationExtractor struct{}

func (compilationExtractor) Extract(path string, info os.FileInfo) (Metadata, error) {
	artist := "First Artist"
	if filepath.Base(path) == "second.mp3" {
		artist = "Second Artist"
	}
	return Metadata{
		Title: filepath.Base(path), Artist: artist, Album: "VA", AlbumArtist: artist,
		Size: info.Size(), Suffix: "mp3", ContentType: "audio/mpeg",
	}, nil
}

func TestScanGroupsInferredMultiArtistFolderAsCompilation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	compilationDir := filepath.Join(root, "Compilation Volume")
	if err := os.Mkdir(compilationDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.mp3", "second.mp3", "third.mp3"} {
		if err := os.WriteFile(filepath.Join(compilationDir, name), []byte("audio"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	if _, err := New(root, catalog, compilationExtractor{}).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	albums, err := catalog.Albums(ctx, ports.AlbumListQuery{Limit: 10})
	if err != nil || len(albums) != 1 {
		t.Fatalf("Albums() = %#v, %v", albums, err)
	}
	if albums[0].Name != "Compilation Volume" || albums[0].Artist != "Various Artists" || albums[0].SongCount != 3 {
		t.Fatalf("album = %#v", albums[0])
	}
	tracks, err := catalog.TracksByAlbum(ctx, albums[0].ID)
	if err != nil || len(tracks) != 3 {
		t.Fatalf("TracksByAlbum() = %#v, %v", tracks, err)
	}
	if tracks[0].ArtistID == tracks[1].ArtistID || tracks[0].AlbumArtistID != albums[0].ArtistID {
		t.Fatalf("track identities = %#v", tracks)
	}
	legacyArtistID := domain.StableID("artist", "first artist")
	legacyAlbumID := domain.StableID("album", legacyArtistID, "va")
	legacyTracks, err := catalog.TracksByAlbum(ctx, legacyAlbumID)
	if err != nil || len(legacyTracks) != 2 || legacyTracks[0].AlbumID != albums[0].ID {
		t.Fatalf("TracksByAlbum(legacy) = %#v, %v", legacyTracks, err)
	}
}
