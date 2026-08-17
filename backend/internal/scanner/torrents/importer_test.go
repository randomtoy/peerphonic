package torrents

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type scanControllerStub struct {
	calls int
}

func (s *scanControllerStub) ScanNow(context.Context) (scanner.Report, error) {
	s.calls++
	return scanner.Report{}, nil
}

func TestImporterStoresMetainfoByInfoHashAndStartsScan(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join(t.TempDir(), "fixture.torrent")
	writeTorrent(t, fixturePath, metainfo.Info{
		Name: "Artist - Album", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{{Length: 100, Path: []string{"Song.mp3"}}},
	})
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "torrents")
	scans := &scanControllerStub{}
	importer := NewImporter(root, torrentprovider.New(), scans)
	result, err := importer.Import(context.Background(), bytes.NewReader(contents))
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.SourceID == "" || result.Name != "Artist - Album" || result.Tracks != 1 || scans.calls != 1 {
		t.Fatalf("result = %#v, scans = %d", result, scans.calls)
	}
	stored, err := os.ReadFile(filepath.Join(root, result.SourceID+".torrent"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, contents) {
		t.Fatal("stored metainfo differs from upload")
	}
}
