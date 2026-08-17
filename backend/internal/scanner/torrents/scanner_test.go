package torrents

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	basescanner "github.com/randomtoy/peerphonic/backend/internal/scanner"
)

func TestScanImportsTorrentAudioMetadataAndReplacesRemovedFiles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	metadataPath := filepath.Join(root, "album.torrent")
	writeTorrent(t, metadataPath, metainfo.Info{
		Name: "Remote Artist - Remote Album", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{
			{Length: 100, Path: []string{"01 First.flac"}},
			{Length: 200, Path: []string{"02 Second.mp3"}},
			{Length: 50, Path: []string{"front.jpg"}},
		},
	})
	if err := os.WriteFile(filepath.Join(root, "broken.torrent"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	scanner := New(root, catalog, torrentprovider.New())
	report, err := scanner.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if report.Tracks != 2 || len(report.Warnings) != 1 {
		t.Fatalf("report = %#v", report)
	}
	albums, err := catalog.Albums(ctx, 0, 10)
	if err != nil || len(albums) != 1 || albums[0].Name != "Remote Album" ||
		albums[0].SongCount != 2 || albums[0].CoverArtID == "" {
		t.Fatalf("Albums() = %#v, %v", albums, err)
	}
	tracks, err := catalog.TracksByAlbum(ctx, albums[0].ID)
	if err != nil || len(tracks) != 2 {
		t.Fatalf("TracksByAlbum() = %#v, %v", tracks, err)
	}
	sources, err := catalog.Sources(ctx, tracks[0].ID)
	if err != nil || len(sources) != 1 || sources[0].Provider != torrentprovider.Name {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}

	if err := os.Remove(metadataPath); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Track(ctx, tracks[0].ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Track() after removal error = %v, want ErrNotFound", err)
	}
}

func TestScanReusesEnrichedMetadataFromCompletedTorrentCache(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	dataRoot := t.TempDir()
	trackBytes := []byte("complete cached track")
	writeTorrent(t, filepath.Join(root, "album.torrent"), metainfo.Info{
		Name: "Remote Artist - Remote Album", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{{Length: int64(len(trackBytes)), Path: []string{"01 Song.mp3"}}},
	})
	mediaPath := filepath.Join(dataRoot, "Remote Artist - Remote Album", "01 Song.mp3")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, trackBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	provider, err := torrentprovider.NewStreaming(root, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewWithEnrichment(root, catalog, provider, enrichmentExtractor{metadata: basescanner.Metadata{
		Title: "Tagged Song", Artist: "Tagged Artist", Album: "Tagged Album",
		AlbumArtist: "Tagged Artist", TrackNumber: 1, Duration: 2 * time.Minute,
		Size: int64(len(trackBytes)), BitRate: 256, Suffix: "mp3", ContentType: "audio/mpeg",
	}}, nil)
	if _, err := scanner.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	albums, err := catalog.Albums(ctx, 0, 10)
	if err != nil || len(albums) != 1 || albums[0].Name != "Remote Album" ||
		albums[0].Duration != 2*time.Minute {
		t.Fatalf("Albums() = %#v, %v", albums, err)
	}
	tracks, err := catalog.TracksByAlbum(ctx, albums[0].ID)
	if err != nil || len(tracks) != 1 || tracks[0].Title != "Tagged Song" || tracks[0].BitRate != 256 {
		t.Fatalf("TracksByAlbum() = %#v, %v", tracks, err)
	}
}

func writeTorrent(t *testing.T, path string, info metainfo.Info) {
	t.Helper()
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	meta := metainfo.MetaInfo{InfoBytes: infoBytes}
	if err := meta.Write(&buffer); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
