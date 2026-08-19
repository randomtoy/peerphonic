package torrents

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type magnetFetcherStub struct {
	contents []byte
	block    bool
	started  chan struct{}
	once     sync.Once
}

func (s *magnetFetcherStub) FetchMagnetMetadata(ctx context.Context, _ string) ([]byte, error) {
	s.once.Do(func() {
		if s.started != nil {
			close(s.started)
		}
	})
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.contents, nil
}

type completedImporterStub struct {
	mu    sync.Mutex
	calls int
}

func (s *completedImporterStub) Import(_ context.Context, source io.Reader) (ports.SourceImportResult, error) {
	if _, err := io.ReadAll(source); err != nil {
		return ports.SourceImportResult{}, err
	}
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return ports.SourceImportResult{SourceID: "source-id", Name: "Artist - Album", Tracks: 3}, nil
}

func TestMagnetImporterCompletesAndPersistsJob(t *testing.T) {
	t.Parallel()

	contents, magnet := magnetFixture(t)
	root := t.TempDir()
	manager, err := NewMagnetImporter(
		context.Background(), root, &magnetFetcherStub{contents: contents}, &completedImporterStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	created, err := manager.ImportURI(context.Background(), magnet)
	if err != nil {
		t.Fatalf("ImportURI() error = %v", err)
	}
	if created.State != domain.SourceImportStateFetching || created.SourceID == "" {
		t.Fatalf("created import = %#v", created)
	}
	completed := waitForImportState(t, manager, domain.SourceImportStateReady)
	if completed.Name != "Artist - Album" || completed.Tracks != 3 || completed.SourceID != "source-id" {
		t.Fatalf("completed import = %#v", completed)
	}
	if _, err := os.Stat(filepath.Join(root, "imports", created.ID+".json")); err != nil {
		t.Fatalf("persisted import job: %v", err)
	}
}

func TestMagnetImporterResumesInterruptedMetadataFetch(t *testing.T) {
	t.Parallel()

	contents, magnet := magnetFixture(t)
	root := t.TempDir()
	started := make(chan struct{})
	first, err := NewMagnetImporter(
		context.Background(), root,
		&magnetFetcherStub{block: true, started: started}, &completedImporterStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.ImportURI(context.Background(), magnet); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("metadata fetch did not start")
	}
	first.Close()

	second, err := NewMagnetImporter(
		context.Background(), root, &magnetFetcherStub{contents: contents}, &completedImporterStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	completed := waitForImportState(t, second, domain.SourceImportStateReady)
	if completed.Tracks != 3 {
		t.Fatalf("resumed import = %#v", completed)
	}
}

func magnetFixture(t *testing.T) ([]byte, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.torrent")
	writeTorrent(t, path, metainfo.Info{
		Name: "Artist - Album", PieceLength: 16 * 1024, Pieces: make([]byte, 20),
		Files: []metainfo.FileInfo{{Length: 100, Path: []string{"Song.mp3"}}},
	})
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := metainfo.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := metadata.UnmarshalInfo()
	if err != nil {
		t.Fatal(err)
	}
	magnet := metadata.Magnet(nil, &info)
	return contents, magnet.String()
}

func waitForImportState(t *testing.T, manager *MagnetImporter, state domain.SourceImportState) domain.SourceImport {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		items, err := manager.SourceImports(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 1 && items[0].State == state {
			return items[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	items, _ := manager.SourceImports(context.Background())
	t.Fatalf("imports did not reach %q: %#v", state, items)
	return domain.SourceImport{}
}
