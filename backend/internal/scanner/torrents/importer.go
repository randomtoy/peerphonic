package torrents

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type scanController interface {
	ScanNow(ctx context.Context) (scanner.Report, error)
}

type Importer struct {
	root     string
	provider *torrentprovider.Provider
	scans    scanController
}

func NewImporter(root string, provider *torrentprovider.Provider, scans scanController) *Importer {
	return &Importer{root: root, provider: provider, scans: scans}
}

func (i *Importer) Import(ctx context.Context, source io.Reader) (ports.SourceImportResult, error) {
	contents, err := io.ReadAll(&contextReader{ctx: ctx, reader: source})
	if err != nil {
		return ports.SourceImportResult{}, fmt.Errorf("read torrent upload: %w", err)
	}
	metadata, err := i.provider.ReadCatalog(bytes.NewReader(contents))
	if err != nil {
		return ports.SourceImportResult{}, err
	}
	if err := os.MkdirAll(i.root, 0o755); err != nil {
		return ports.SourceImportResult{}, fmt.Errorf("create torrent metadata directory: %w", err)
	}
	target := filepath.Join(i.root, metadata.InfoHash+".torrent")
	temporary, err := os.CreateTemp(i.root, ".peerphonic-torrent-*")
	if err != nil {
		return ports.SourceImportResult{}, fmt.Errorf("create temporary torrent metadata: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return ports.SourceImportResult{}, fmt.Errorf("write torrent metadata: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return ports.SourceImportResult{}, fmt.Errorf("sync torrent metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return ports.SourceImportResult{}, fmt.Errorf("close torrent metadata: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return ports.SourceImportResult{}, fmt.Errorf("commit torrent metadata: %w", err)
	}
	if _, err := i.scans.ScanNow(ctx); err != nil {
		return ports.SourceImportResult{}, fmt.Errorf("scan imported torrent metadata: %w", err)
	}
	return ports.SourceImportResult{
		SourceID: metadata.InfoHash, Name: metadata.Name, Tracks: len(metadata.Tracks),
	}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
