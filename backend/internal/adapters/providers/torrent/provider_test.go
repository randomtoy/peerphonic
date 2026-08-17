package torrent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestReadCatalogBuildsProvisionalAudioMetadata(t *testing.T) {
	t.Parallel()

	data := torrentBytes(t, metainfo.Info{
		Name: "Example Artist - Example Album", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{
			{Length: 123, Path: []string{"01 - First Song.flac"}},
			{Length: 456, Path: []string{"02 Second Song.mp3"}},
			{Length: 50, Path: []string{"cover.jpg"}},
		},
	})
	catalog, err := New().ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadCatalog() error = %v", err)
	}
	if catalog.InfoHash == "" || catalog.Name != "Example Artist - Example Album" {
		t.Fatalf("catalog = %#v", catalog)
	}
	if len(catalog.Tracks) != 2 {
		t.Fatalf("tracks = %#v", catalog.Tracks)
	}
	first := catalog.Tracks[0]
	if first.Track.Title != "First Song" || first.Track.TrackNumber != 1 ||
		first.Track.Artist != "Example Artist" || first.Track.Album != "Example Album" ||
		first.Track.Size != 123 || first.Track.CoverArtID == "" || first.Ref.Provider != Name ||
		!strings.Contains(first.Ref.Key, "Example Artist - Example Album/01 - First Song.flac") {
		t.Fatalf("first track = %#v", first)
	}
}

func TestReadCatalogRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	data := torrentBytes(t, metainfo.Info{
		Name: "Album", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{{Length: 1, Path: []string{"..", "song.mp3"}}},
	})
	if _, err := New().ReadCatalog(bytes.NewReader(data)); err == nil {
		t.Fatal("ReadCatalog() error = nil, want unsafe path error")
	}
}

func TestReadCatalogSupportsSingleFileTorrent(t *testing.T) {
	t.Parallel()

	data := torrentBytes(t, metainfo.Info{
		Name: "Solo Artist - Solo Song.mp3", Length: 321,
		PieceLength: 16 * 1024, Pieces: make([]byte, 20),
	})
	catalog, err := New().ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Tracks) != 1 || catalog.Tracks[0].Track.Title != "Solo Song" ||
		catalog.Tracks[0].Track.Artist != "Solo Artist" || catalog.Tracks[0].Track.Album != "Unknown Album" {
		t.Fatalf("tracks = %#v", catalog.Tracks)
	}
}

func TestResolveReportsMetadataOnlySourceAsUnavailable(t *testing.T) {
	t.Parallel()

	_, err := New().Resolve(context.Background(), domain.SourceRef{
		Provider: Name, Key: strings.Repeat("0", 40) + "/Album/song.mp3",
	})
	if !errors.Is(err, ports.ErrSourceUnavailable) {
		t.Fatalf("Resolve() error = %v, want ErrSourceUnavailable", err)
	}
}

func TestCacheUsageIncludesCompletePartialAndStateFiles(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	provider, err := NewStreaming(t.TempDir(), dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"Album/track.mp3":     bytes.Repeat([]byte("a"), 4096),
		"Album/next.mp3.part": bytes.Repeat([]byte("b"), 8192),
		".torrent.db":         bytes.Repeat([]byte("d"), 4096),
	}
	for name, contents := range files {
		filePath := filepath.Join(dataRoot, name)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := provider.CacheUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.Name != Name || usage.Entries != 2 || usage.PartialEntries != 1 || usage.Size < 12*1024 {
		t.Fatalf("CacheUsage() = %#v", usage)
	}
}

func TestCacheUsageAllowsMissingDataDirectory(t *testing.T) {
	t.Parallel()

	provider, err := NewStreaming(t.TempDir(), filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	usage, err := provider.CacheUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.Name != Name || usage.Size != 0 || usage.Entries != 0 {
		t.Fatalf("CacheUsage() = %#v", usage)
	}
}

func TestEvictRemovesOldestInactiveCachedTorrentFile(t *testing.T) {
	t.Parallel()

	data := torrentBytes(t, metainfo.Info{
		Name: "Artist - Album", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{
			{Length: 4096, Path: []string{"01 Old.mp3"}},
			{Length: 4096, Path: []string{"02 New.mp3"}},
		},
	})
	dataRoot := t.TempDir()
	provider, err := NewStreaming(t.TempDir(), dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := provider.ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dataRoot, catalog.Name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01 Old.mp3", "02 New.mp3"} {
		if err := os.WriteFile(filepath.Join(root, name), bytes.Repeat([]byte("x"), 4096), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	provider.cacheMu.Lock()
	provider.lastAccessed[catalog.Tracks[0].Ref.Key] = time.Unix(1, 0)
	provider.lastAccessed[catalog.Tracks[1].Ref.Key] = time.Unix(2, 0)
	provider.cacheMu.Unlock()

	freed, err := provider.Evict(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if freed <= 0 {
		t.Fatalf("Evict() freed = %d", freed)
	}
	if _, err := os.Stat(filepath.Join(root, "01 Old.mp3")); !os.IsNotExist(err) {
		t.Fatalf("old file stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(root, "02 New.mp3")); err != nil {
		t.Fatalf("new file stat error = %v", err)
	}
}

func TestEvictSkipsActiveTorrent(t *testing.T) {
	t.Parallel()

	data := torrentBytes(t, metainfo.Info{
		Name: "Artist - Active.mp3", Length: 4096, PieceLength: 16 * 1024, Pieces: make([]byte, 20),
	})
	dataRoot := t.TempDir()
	provider, err := NewStreaming(t.TempDir(), dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := provider.ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(dataRoot, catalog.Name)
	if err := os.WriteFile(filePath, bytes.Repeat([]byte("x"), 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	provider.retain(catalog.InfoHash, catalog.Tracks[0].Ref.Key)
	freed, err := provider.Evict(context.Background(), 4096)
	provider.release(catalog.InfoHash)
	if err != nil {
		t.Fatal(err)
	}
	if freed != 0 {
		t.Fatalf("Evict() freed = %d, want 0", freed)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("active file stat error = %v", err)
	}
}

func TestStreamingProviderReadsPersistedTorrentFileAndArtwork(t *testing.T) {
	t.Parallel()

	sourceRoot := filepath.Join(t.TempDir(), "Example Artist - Example Album")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	media := bytes.Repeat([]byte("peerphonic-audio"), 4096)
	artwork := []byte{0xff, 0xd8, 0xff, 0xe0, 'J', 'F', 'I', 'F'}
	if err := os.WriteFile(filepath.Join(sourceRoot, "01 Song.mp3"), media, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "front.jpg"), artwork, 0o600); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(sourceRoot); err != nil {
		t.Fatal(err)
	}
	data := torrentBytes(t, info)
	metadataRoot := t.TempDir()
	dataRoot := t.TempDir()
	provider, err := NewStreaming(metadataRoot, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	completedFiles := make(chan CompletedFile, 1)
	provider.SetCompletedHandler(func(file CompletedFile) { completedFiles <- file })
	catalog, err := provider.ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Tracks) != 1 || catalog.Tracks[0].Track.CoverArtID == "" {
		t.Fatalf("catalog = %#v", catalog)
	}
	if provider.client != nil {
		t.Fatal("torrent client started while reading metadata")
	}
	if err := os.WriteFile(filepath.Join(metadataRoot, catalog.InfoHash+".torrent"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	targetRoot := filepath.Join(dataRoot, filepath.Base(sourceRoot))
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01 Song.mp3", "front.jpg"} {
		contents, err := os.ReadFile(filepath.Join(sourceRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(targetRoot, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resolved, err := provider.Resolve(ctx, catalog.Tracks[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := io.ReadAll(resolved.Content)
	resolved.Content.Close()
	if err != nil || !bytes.Equal(streamed, media) {
		t.Fatalf("streamed %d bytes, err = %v", len(streamed), err)
	}
	select {
	case completed := <-completedFiles:
		if completed.TrackID != catalog.Tracks[0].Track.ID ||
			completed.Path != filepath.Join(targetRoot, "01 Song.mp3") {
			t.Fatalf("completed file = %#v", completed)
		}
	case <-time.After(time.Second):
		t.Fatal("completed file callback was not called")
	}
	ranged, err := provider.Resolve(ctx, catalog.Tracks[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ranged.Content.Seek(11, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 16)
	if _, err := io.ReadFull(ranged.Content, buffer); err != nil {
		t.Fatal(err)
	}
	ranged.Content.Close()
	if !bytes.Equal(buffer, media[11:27]) {
		t.Fatalf("range bytes = %q, want %q", buffer, media[11:27])
	}
	select {
	case completed := <-completedFiles:
		t.Fatalf("range read completed file = %#v", completed)
	case <-time.After(50 * time.Millisecond):
	}
	cover, err := provider.OpenArtwork(ctx, catalog.Tracks[0].Track.CoverArtID)
	if err != nil {
		t.Fatal(err)
	}
	streamedArtwork, err := io.ReadAll(cover.Content)
	cover.Content.Close()
	if err != nil || !bytes.Equal(streamedArtwork, artwork) {
		t.Fatalf("artwork = %x, err = %v", streamedArtwork, err)
	}
	provider.cacheMu.Lock()
	provider.lastAccessed[catalog.Tracks[0].Ref.Key] = time.Unix(1, 0)
	provider.cacheMu.Unlock()
	freed, err := provider.Evict(context.Background(), 1)
	if err != nil || freed <= 0 {
		t.Fatalf("Evict() freed = %d, error = %v", freed, err)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "01 Song.mp3")); !os.IsNotExist(err) {
		t.Fatalf("evicted track stat error = %v, want not exist", err)
	}
	hash := metainfo.NewHashFromHex(catalog.InfoHash)
	if _, ok := provider.client.Torrent(hash); ok {
		t.Fatal("torrent remained attached after cache eviction")
	}
	reopened, err := provider.Resolve(ctx, catalog.Tracks[0].Ref)
	if err != nil {
		t.Fatalf("Resolve() after eviction error = %v", err)
	}
	reopened.Content.Close()
	torrent, ok := provider.client.Torrent(hash)
	if !ok {
		t.Fatal("torrent was not reattached after cache eviction")
	}
	if torrent.BytesCompleted() >= info.TotalLength() {
		t.Fatalf("torrent completed bytes after eviction = %d", torrent.BytesCompleted())
	}
}

func torrentBytes(t *testing.T, info metainfo.Info) []byte {
	t.Helper()
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	meta := metainfo.MetaInfo{InfoBytes: infoBytes}
	var buffer bytes.Buffer
	if err := meta.Write(&buffer); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
