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

	torrentclient "github.com/anacrolix/torrent"
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

func TestReadCatalogUsesScanAsArtworkFallback(t *testing.T) {
	t.Parallel()

	data := torrentBytes(t, metainfo.Info{
		Name: "Artist", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{
			{Length: 123, Path: []string{"Album", "01 Song.mp3"}},
			{Length: 50, Path: []string{"Album", "Scans", "img002.jpg"}},
			{Length: 50, Path: []string{"Album", "Scans", "img001.jpg"}},
		},
	})
	provider := New()
	catalog, err := provider.ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Tracks) != 1 || catalog.Tracks[0].Track.CoverArtID == "" {
		t.Fatalf("tracks = %#v", catalog.Tracks)
	}
	provider.artworkMu.RLock()
	artwork := provider.artworks[catalog.Tracks[0].Track.CoverArtID]
	provider.artworkMu.RUnlock()
	if !strings.HasSuffix(artwork.Key, "/Artist/Album/Scans/img001.jpg") {
		t.Fatalf("artwork = %#v", artwork)
	}
}

func TestReadCatalogPrefersFrontArtworkOverFallback(t *testing.T) {
	t.Parallel()

	data := torrentBytes(t, metainfo.Info{
		Name: "Artist", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{
			{Length: 123, Path: []string{"Album", "01 Song.mp3"}},
			{Length: 50, Path: []string{"Album", "random.jpg"}},
			{Length: 50, Path: []string{"Album", "Scans", "front.jpg"}},
		},
	})
	provider := New()
	catalog, err := provider.ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	provider.artworkMu.RLock()
	artwork := provider.artworks[catalog.Tracks[0].Track.CoverArtID]
	provider.artworkMu.RUnlock()
	if !strings.HasSuffix(artwork.Key, "/Artist/Album/Scans/front.jpg") {
		t.Fatalf("artwork = %#v", artwork)
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

func TestReadCatalogRecognizesDiscographySections(t *testing.T) {
	t.Parallel()

	data := torrentBytes(t, metainfo.Info{
		Name: "Группа Руки Вверх!", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{{
			Length: 123,
			Path: []string{
				"1. Альбомы", "1997. Дышите равномерно", "CD 2", "01 - Доброе утро.mp3",
			},
		}},
	})
	catalog, err := New().ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Tracks) != 1 {
		t.Fatalf("tracks = %#v", catalog.Tracks)
	}
	track := catalog.Tracks[0].Track
	if track.Artist != "Руки Вверх!" || track.Album != "1997. Дышите равномерно" ||
		track.DiscNumber != 2 || track.Year != 1997 {
		t.Fatalf("track = %#v", track)
	}
}

func TestProvisionalArtistAlbumSupportsCollectionAndArtistRoots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		parts      []string
		artist     string
		album      string
		discNumber int
	}{
		{
			name:   "artist root with section",
			parts:  []string{"Artist", "Albums", "2001 - Album", "01 Song.mp3"},
			artist: "Artist", album: "2001 - Album",
		},
		{
			name:   "collection root with artist",
			parts:  []string{"Music Collection", "Artist", "Album", "01 Song.mp3"},
			artist: "Artist", album: "Album",
		},
		{
			name:   "collection root with artist section",
			parts:  []string{"Music Collection", "Artist", "Studio Albums", "Album", "Disc 1", "01 Song.mp3"},
			artist: "Artist", album: "Album", discNumber: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artist, album, discNumber := provisionalArtistAlbum(test.parts)
			if artist != test.artist || album != test.album || discNumber != test.discNumber {
				t.Fatalf("provisionalArtistAlbum() = %q, %q, %d", artist, album, discNumber)
			}
		})
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

func TestApplyTransferLimits(t *testing.T) {
	t.Parallel()

	clientConfig := torrentclient.NewDefaultClientConfig()
	applyTransferLimits(clientConfig, StreamingOptions{UploadLimit: 1024, DownloadLimit: 2048})
	if got := int64(clientConfig.UploadRateLimiter.Limit()); got != 1024 {
		t.Fatalf("upload limit = %d, want 1024", got)
	}
	if clientConfig.UploadRateLimiter.Burst() < 256<<10 {
		t.Fatalf("upload burst = %d, want at least %d", clientConfig.UploadRateLimiter.Burst(), 256<<10)
	}
	if got := int64(clientConfig.DownloadRateLimiter.Limit()); got != 2048 {
		t.Fatalf("download limit = %d, want 2048", got)
	}
}

func TestReadCatalogIsolatesLegacyCacheByInfoHash(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	provider, err := NewStreaming(t.TempDir(), dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(dataRoot, "Same Name.mp3")
	if err := os.WriteFile(legacyPath, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := provider.ReadCatalog(bytes.NewReader(torrentBytes(t, metainfo.Info{
		Name: "Same Name.mp3", Length: 5, PieceLength: 16 * 1024, Pieces: bytes.Repeat([]byte{1}, 20),
	})))
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.ReadCatalog(bytes.NewReader(torrentBytes(t, metainfo.Info{
		Name: "Same Name.mp3", Length: 6, PieceLength: 16 * 1024, Pieces: bytes.Repeat([]byte{2}, 20),
	})))
	if err != nil {
		t.Fatal(err)
	}
	if first.InfoHash == second.InfoHash {
		t.Fatal("different torrents have the same info hash")
	}
	if _, err := os.Stat(provider.cachePath(first.InfoHash, "Same Name.mp3")); err != nil {
		t.Fatalf("first isolated cache stat error = %v", err)
	}
	if _, err := os.Stat(provider.cachePath(second.InfoHash, "Same Name.mp3")); !os.IsNotExist(err) {
		t.Fatalf("second isolated cache stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(provider.cachePath(second.InfoHash, "Same Name.mp3") + ".part"); !os.IsNotExist(err) {
		t.Fatalf("second isolated partial cache stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy cache stat error = %v, want not exist", err)
	}
}

func TestResolveRejectsCorruptLegacyCachePieces(t *testing.T) {
	t.Parallel()

	sourcePath := filepath.Join(t.TempDir(), "Track.mp3")
	wanted := bytes.Repeat([]byte("wanted-audio"), 4096)
	if err := os.WriteFile(sourcePath, wanted, 0o600); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(sourcePath); err != nil {
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
	if err := os.WriteFile(filepath.Join(dataRoot, info.Name), bytes.Repeat([]byte("x"), len(wanted)), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := provider.ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataRoot, catalog.InfoHash+".torrent"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := provider.Resolve(context.Background(), catalog.Tracks[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Content.Close()
	hash := metainfo.NewHashFromHex(catalog.InfoHash)
	torrent, ok := provider.client.Torrent(hash)
	if !ok {
		t.Fatal("torrent was not attached")
	}
	if completed := torrent.BytesCompleted(); completed != 0 {
		t.Fatalf("corrupt legacy cache completed bytes = %d, want 0", completed)
	}
	if _, err := os.Stat(provider.cachePath(catalog.InfoHash, info.Name) + ".part"); err != nil {
		t.Fatalf("corrupt cache partial stat error = %v", err)
	}
}

func TestStreamingProviderReusesVerifiedCacheAfterRestart(t *testing.T) {
	t.Parallel()

	sourcePath := filepath.Join(t.TempDir(), "Cached Track.mp3")
	media := bytes.Repeat([]byte("cached-audio"), 4096)
	if err := os.WriteFile(sourcePath, media, 0o600); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(sourcePath); err != nil {
		t.Fatal(err)
	}
	data := torrentBytes(t, info)
	metadataRoot := t.TempDir()
	dataRoot := t.TempDir()
	provider, err := NewStreaming(metadataRoot, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := provider.ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataRoot, catalog.InfoHash+".torrent"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, info.Name), media, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resolved, err := provider.Resolve(ctx, catalog.Tracks[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	firstRead, err := io.ReadAll(resolved.Content)
	resolved.Content.Close()
	if err != nil || !bytes.Equal(firstRead, media) {
		t.Fatalf("first read bytes = %d, error = %v", len(firstRead), err)
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStreaming(metadataRoot, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	resolved, err = reopened.Resolve(ctx, catalog.Tracks[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	secondRead, err := io.ReadAll(resolved.Content)
	resolved.Content.Close()
	if err != nil || !bytes.Equal(secondRead, media) {
		t.Fatalf("reopened read bytes = %d, error = %v", len(secondRead), err)
	}
}

func TestResolvePrefersWorkingPartialCacheOverStaleCompleteFile(t *testing.T) {
	t.Parallel()

	sourcePath := filepath.Join(t.TempDir(), "Complete Track.mp3")
	media := bytes.Repeat([]byte("complete-audio"), 4096)
	if err := os.WriteFile(sourcePath, media, 0o600); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(sourcePath); err != nil {
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
	catalog, err := provider.ReadCatalog(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataRoot, catalog.InfoHash+".torrent"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	completePath := provider.cachePath(catalog.InfoHash, info.Name)
	if err := os.MkdirAll(filepath.Dir(completePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(completePath, bytes.Repeat([]byte("x"), len(media)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(completePath+".part", media, 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := provider.Resolve(context.Background(), catalog.Tracks[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := io.ReadAll(resolved.Content)
	resolved.Content.Close()
	if err != nil || !bytes.Equal(streamed, media) {
		t.Fatalf("streamed %d bytes, error = %v", len(streamed), err)
	}
	if _, err := os.Stat(completePath + ".part"); err == nil {
		if _, err := os.Stat(completePath); !os.IsNotExist(err) {
			t.Fatalf("stale complete cache stat error = %v, want not exist", err)
		}
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
	root := filepath.Join(dataRoot, catalog.InfoHash, catalog.Name)
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
	filePath := filepath.Join(dataRoot, catalog.InfoHash, catalog.Name)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
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
	transfers, err := provider.Transfers(context.Background())
	if err != nil || len(transfers) != 0 {
		t.Fatalf("Transfers() before open = %#v, %v", transfers, err)
	}
	if err := os.WriteFile(filepath.Join(metadataRoot, catalog.InfoHash+".torrent"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyRoot := filepath.Join(dataRoot, filepath.Base(sourceRoot))
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01 Song.mp3", "front.jpg"} {
		contents, err := os.ReadFile(filepath.Join(sourceRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(legacyRoot, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	targetRoot := filepath.Join(dataRoot, catalog.InfoHash, filepath.Base(sourceRoot))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resolved, err := provider.Resolve(ctx, catalog.Tracks[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	transfers, err = provider.Transfers(ctx)
	if err != nil || len(transfers) != 1 || transfers[0].Provider != Name ||
		transfers[0].ID != catalog.InfoHash || transfers[0].Name != info.Name ||
		transfers[0].TotalBytes != info.TotalLength() || transfers[0].ActiveStreams != 1 {
		t.Fatalf("Transfers() while open = %#v, %v", transfers, err)
	}
	streamed, err := io.ReadAll(resolved.Content)
	resolved.Content.Close()
	if err != nil || !bytes.Equal(streamed, media) {
		t.Fatalf("streamed %d bytes, err = %v", len(streamed), err)
	}
	transfers, err = provider.Transfers(ctx)
	if err != nil || len(transfers) != 1 || transfers[0].CompletedBytes < int64(len(media)) ||
		transfers[0].CompletedBytes > transfers[0].TotalBytes || transfers[0].ActiveStreams != 0 {
		t.Fatalf("Transfers() after read = %#v, %v", transfers, err)
	}
	if _, err := os.Stat(filepath.Join(legacyRoot, "01 Song.mp3")); !os.IsNotExist(err) {
		t.Fatalf("legacy track stat error = %v, want not exist after migration", err)
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
