package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
)

type extractorStub struct{}

func (extractorStub) Extract(path string, info os.FileInfo) (Metadata, error) {
	return Metadata{
		Title: filepath.Base(path), Artist: "Track Artist", Album: "Album",
		AlbumArtist: "Album Artist", Size: info.Size(), Suffix: "mp3", ContentType: "audio/mpeg",
	}, nil
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

	s := New(root, catalog, extractorStub{})
	report, err := s.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if report.Tracks != 2 || len(report.Warnings) != 0 {
		t.Fatalf("Scan() report = %#v", report)
	}
	artists, err := catalog.Artists(ctx)
	if err != nil || len(artists) != 1 {
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
