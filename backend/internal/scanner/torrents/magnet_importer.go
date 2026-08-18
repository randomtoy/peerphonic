package torrents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

const scanRetryInterval = 500 * time.Millisecond

type magnetMetadataFetcher interface {
	FetchMagnetMetadata(ctx context.Context, uri string) ([]byte, error)
}

type magnetImportRecord struct {
	Import domain.SourceImport `json:"import"`
	URI    string              `json:"uri"`
}

// MagnetImporter owns asynchronous magnet metadata retrieval and delegates
// the resulting payload to the regular source importer.
type MagnetImporter struct {
	ctx      context.Context
	cancel   context.CancelFunc
	root     string
	fetcher  magnetMetadataFetcher
	importer ports.SourceImporter

	mu     sync.Mutex
	jobs   map[string]magnetImportRecord
	active map[string]struct{}
	wg     sync.WaitGroup
}

func NewMagnetImporter(
	ctx context.Context,
	root string,
	fetcher magnetMetadataFetcher,
	importer ports.SourceImporter,
) (*MagnetImporter, error) {
	workerContext, cancel := context.WithCancel(ctx)
	manager := &MagnetImporter{
		ctx: workerContext, cancel: cancel, root: filepath.Join(root, "imports"),
		fetcher: fetcher, importer: importer,
		jobs: make(map[string]magnetImportRecord), active: make(map[string]struct{}),
	}
	if err := manager.load(); err != nil {
		cancel()
		return nil, err
	}
	manager.mu.Lock()
	for id, record := range manager.jobs {
		if record.Import.State == domain.SourceImportStateFetching ||
			record.Import.State == domain.SourceImportStateScanning {
			manager.startLocked(id)
		}
	}
	manager.mu.Unlock()
	return manager, nil
}

func (m *MagnetImporter) ImportURI(ctx context.Context, uri string) (domain.SourceImport, error) {
	if err := ctx.Err(); err != nil {
		return domain.SourceImport{}, err
	}
	uri = strings.TrimSpace(uri)
	magnet, err := metainfo.ParseMagnetUri(uri)
	if err != nil {
		return domain.SourceImport{}, fmt.Errorf("parse magnet URI: %w", err)
	}
	id := magnet.InfoHash.HexString()
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.jobs[id]; ok && existing.Import.State != domain.SourceImportStateFailed {
		return existing.Import, nil
	}
	createdAt := now
	if existing, ok := m.jobs[id]; ok && !existing.Import.CreatedAt.IsZero() {
		createdAt = existing.Import.CreatedAt
	}
	record := magnetImportRecord{
		Import: domain.SourceImport{
			ID: id, Provider: torrentprovider.Name, Name: strings.TrimSpace(magnet.DisplayName),
			SourceID: id, State: domain.SourceImportStateFetching,
			CreatedAt: createdAt, UpdatedAt: now,
		},
		URI: uri,
	}
	m.jobs[id] = record
	if err := m.persistLocked(record); err != nil {
		delete(m.jobs, id)
		return domain.SourceImport{}, err
	}
	m.startLocked(id)
	return record.Import, nil
}

func (m *MagnetImporter) SourceImports(ctx context.Context) ([]domain.SourceImport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]domain.SourceImport, 0, len(m.jobs))
	for _, record := range m.jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result = append(result, record.Import)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func (m *MagnetImporter) Close() {
	m.cancel()
	m.wg.Wait()
}

func (m *MagnetImporter) startLocked(id string) {
	if _, running := m.active[id]; running {
		return
	}
	m.active[id] = struct{}{}
	m.wg.Add(1)
	go m.run(id)
}

func (m *MagnetImporter) run(id string) {
	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		delete(m.active, id)
		m.mu.Unlock()
	}()

	m.mu.Lock()
	record, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	contents, err := m.fetcher.FetchMagnetMetadata(m.ctx, record.URI)
	if err != nil {
		if m.ctx.Err() != nil {
			return
		}
		m.fail(id, fmt.Errorf("fetch torrent metadata: %w", err))
		return
	}
	if err := m.setState(id, domain.SourceImportStateScanning); err != nil {
		m.fail(id, err)
		return
	}

	for {
		result, err := m.importer.Import(m.ctx, bytes.NewReader(contents))
		if err == nil {
			m.complete(id, result)
			return
		}
		if m.ctx.Err() != nil {
			return
		}
		if !errors.Is(err, scanner.ErrScanInProgress) {
			m.fail(id, fmt.Errorf("import fetched torrent metadata: %w", err))
			return
		}
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(scanRetryInterval):
		}
	}
}

func (m *MagnetImporter) setState(id string, state domain.SourceImportState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.jobs[id]
	if !ok {
		return ports.ErrNotFound
	}
	record.Import.State = state
	record.Import.Error = ""
	record.Import.UpdatedAt = time.Now().UTC()
	m.jobs[id] = record
	return m.persistLocked(record)
}

func (m *MagnetImporter) complete(id string, result ports.SourceImportResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.jobs[id]
	if !ok {
		return
	}
	record.Import.State = domain.SourceImportStateReady
	record.Import.Error = ""
	record.Import.SourceID = result.SourceID
	record.Import.Name = result.Name
	record.Import.Tracks = result.Tracks
	record.Import.UpdatedAt = time.Now().UTC()
	m.jobs[id] = record
	_ = m.persistLocked(record)
}

func (m *MagnetImporter) fail(id string, cause error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.jobs[id]
	if !ok {
		return
	}
	record.Import.State = domain.SourceImportStateFailed
	record.Import.Error = cause.Error()
	record.Import.UpdatedAt = time.Now().UTC()
	m.jobs[id] = record
	_ = m.persistLocked(record)
}

func (m *MagnetImporter) load() error {
	entries, err := os.ReadDir(m.root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read magnet import jobs: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(m.root, entry.Name()))
		if err != nil {
			return fmt.Errorf("read magnet import job %q: %w", entry.Name(), err)
		}
		var record magnetImportRecord
		if err := json.Unmarshal(contents, &record); err != nil {
			return fmt.Errorf("decode magnet import job %q: %w", entry.Name(), err)
		}
		if record.Import.ID == "" || record.URI == "" {
			return fmt.Errorf("magnet import job %q is incomplete", entry.Name())
		}
		m.jobs[record.Import.ID] = record
	}
	return nil
}

func (m *MagnetImporter) persistLocked(record magnetImportRecord) error {
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return fmt.Errorf("create magnet import job directory: %w", err)
	}
	temporary, err := os.CreateTemp(m.root, ".peerphonic-import-*")
	if err != nil {
		return fmt.Errorf("create temporary magnet import job: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary magnet import job: %w", err)
	}
	if err := json.NewEncoder(temporary).Encode(record); err != nil {
		temporary.Close()
		return fmt.Errorf("encode magnet import job: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync magnet import job: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close magnet import job: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(m.root, record.Import.ID+".json")); err != nil {
		return fmt.Errorf("commit magnet import job: %w", err)
	}
	return nil
}
