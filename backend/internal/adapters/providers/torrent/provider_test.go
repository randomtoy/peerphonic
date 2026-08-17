package torrent

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

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
		first.Track.Size != 123 || first.Ref.Provider != Name ||
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

	_, err := New().Resolve(context.Background(), domain.SourceRef{Provider: Name, Key: "hash/song.mp3"})
	if !errors.Is(err, ports.ErrSourceUnavailable) {
		t.Fatalf("Resolve() error = %v, want ErrSourceUnavailable", err)
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
