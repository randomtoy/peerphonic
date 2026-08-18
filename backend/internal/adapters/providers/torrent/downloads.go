package torrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	torrentclient "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

const downloadProgressInterval = time.Second

type downloadRecord struct {
	Download    domain.TrackDownload `json:"download"`
	Source      domain.SourceRef     `json:"source"`
	LogicalPath string               `json:"logicalPath"`
}

// TrackDownloads returns selected-track cache jobs. A job remains visible
// after completion so the dashboard can distinguish cached and evicted media.
func (p *Provider) TrackDownloads(ctx context.Context) ([]domain.TrackDownload, error) {
	p.downloadMu.Lock()
	defer p.downloadMu.Unlock()
	result := make([]domain.TrackDownload, 0, len(p.downloads))
	for id, record := range p.downloads {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file := p.activeDownloads[id]; file != nil {
			record.Download.CompletedBytes = min(file.BytesCompleted(), record.Download.TotalBytes)
			record.Download.UpdatedAt = time.Now().UTC()
			if record.Download.CompletedBytes >= record.Download.TotalBytes {
				record.Download.State = domain.DownloadStateCached
				record.Download.Error = ""
				if err := p.persistDownloadLocked(record); err != nil {
					return nil, err
				}
			}
			p.downloads[id] = record
		} else if record.Download.State == domain.DownloadStateCached &&
			!fileExists(p.cachePath(record.Download.SourceID, record.LogicalPath)) {
			record.Download.State = domain.DownloadStateEvicted
			record.Download.CompletedBytes = 0
			record.Download.UpdatedAt = time.Now().UTC()
			p.downloads[id] = record
			if err := p.persistDownloadLocked(record); err != nil {
				return nil, err
			}
		}
		result = append(result, record.Download)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].Name < result[j].Name
		}
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result, nil
}

// ResumeTrackDownloads restores unfinished jobs without waiting for another
// playback request. Invalid individual records are marked failed and returned
// as a joined diagnostic error while other jobs continue to resume.
func (p *Provider) ResumeTrackDownloads(ctx context.Context) error {
	p.operation.RLock()
	defer p.operation.RUnlock()
	entries, err := os.ReadDir(p.downloadDirectory())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read torrent download jobs: %w", err)
	}
	var resumeErrors []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(resumeErrors, err)...)
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := p.readDownloadRecord(filepath.Join(p.downloadDirectory(), entry.Name()))
		if err != nil {
			resumeErrors = append(resumeErrors, err)
			continue
		}
		p.downloadMu.Lock()
		if _, exists := p.downloads[record.Download.ID]; !exists {
			p.downloads[record.Download.ID] = record
			_ = p.persistDownloadLocked(record)
		}
		p.downloadMu.Unlock()
		if record.Download.TrackID != "" {
			p.registerTrack(record.Source.Key, record.Download.TrackID)
		}
		if record.Download.State != domain.DownloadStateDownloading ||
			p.sourceMarkerExists(record.Download.SourceID, "paused") {
			continue
		}
		if err := p.resumeTrackDownload(ctx, record); err != nil {
			p.markDownloadFailed(record.Download.ID, err)
			resumeErrors = append(resumeErrors, fmt.Errorf("resume %s: %w", record.Download.Name, err))
		}
	}
	return errors.Join(resumeErrors...)
}

func (p *Provider) queueTrackDownload(
	ref domain.SourceRef,
	infoHash, logicalPath string,
	file *torrentclient.File,
) {
	p.completionMu.Lock()
	trackID := p.trackIDs[ref.Key]
	p.completionMu.Unlock()
	if trackID == "" {
		trackID = domain.StableID("track", Name, infoHash, logicalPath)
		p.registerTrack(ref.Key, trackID)
	}
	now := time.Now().UTC()
	id := domain.StableID("download", ref.Provider, ref.Key)
	record := downloadRecord{
		Download: domain.TrackDownload{
			ID: id, Provider: Name, SourceID: infoHash, TrackID: trackID,
			Name: filepath.Base(logicalPath), State: domain.DownloadStateDownloading,
			CompletedBytes: min(file.BytesCompleted(), file.Length()), TotalBytes: file.Length(),
			StartedAt: now, UpdatedAt: now,
		},
		Source: ref, LogicalPath: logicalPath,
	}
	p.downloadMu.Lock()
	if existing, ok := p.downloads[id]; ok {
		record.Download.StartedAt = existing.Download.StartedAt
		if record.Download.TrackID == "" {
			record.Download.TrackID = existing.Download.TrackID
		}
	}
	if p.activeDownloads[id] != nil {
		p.downloadMu.Unlock()
		return
	}
	p.beginTrackDownloadLocked(record, file)
	p.downloadMu.Unlock()
}

func (p *Provider) beginTrackDownloadLocked(record downloadRecord, file *torrentclient.File) {
	id := record.Download.ID
	completed := min(file.BytesCompleted(), file.Length())
	record.Download.CompletedBytes = completed
	record.Download.TotalBytes = file.Length()
	record.Download.UpdatedAt = time.Now().UTC()
	if completed >= file.Length() {
		record.Download.State = domain.DownloadStateCached
		record.Download.Error = ""
		p.downloads[id] = record
		_ = p.persistDownloadLocked(record)
		return
	}
	record.Download.State = domain.DownloadStateDownloading
	record.Download.Error = ""
	p.completionMu.Lock()
	delete(p.completed, record.Source.Key)
	p.completionMu.Unlock()
	p.downloads[id] = record
	p.activeDownloads[id] = file
	_ = p.persistDownloadLocked(record)
	file.Download()
	p.retain(record.Download.SourceID, record.Source.Key)
	p.downloadWG.Add(1)
	go p.monitorTrackDownload(record.Download.ID, file)
}

func (p *Provider) monitorTrackDownload(id string, file *torrentclient.File) {
	defer p.downloadWG.Done()
	ticker := time.NewTicker(downloadProgressInterval)
	defer ticker.Stop()
	defer func() {
		p.downloadMu.Lock()
		delete(p.activeDownloads, id)
		record := p.downloads[id]
		p.downloadMu.Unlock()
		p.release(record.Download.SourceID)
	}()
	for {
		completed := min(file.BytesCompleted(), file.Length())
		p.downloadMu.Lock()
		record, exists := p.downloads[id]
		if !exists {
			p.downloadMu.Unlock()
			return
		}
		record.Download.CompletedBytes = completed
		record.Download.UpdatedAt = time.Now().UTC()
		if completed >= file.Length() {
			record.Download.State = domain.DownloadStateCached
			record.Download.Error = ""
			p.downloads[id] = record
			_ = p.persistDownloadLocked(record)
			p.downloadMu.Unlock()
			p.notifyCompleted(record.Source, record.LogicalPath)
			return
		}
		p.downloads[id] = record
		p.downloadMu.Unlock()
		select {
		case <-p.downloadCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Provider) resumeTrackDownload(ctx context.Context, record downloadRecord) error {
	meta, err := metainfo.LoadFromFile(filepath.Join(p.metadataRoot, record.Download.SourceID+".torrent"))
	if err != nil {
		return fmt.Errorf("load torrent metadata: %w", err)
	}
	if meta.HashInfoBytes().HexString() != record.Download.SourceID {
		return fmt.Errorf("torrent metadata hash does not match download source")
	}
	client, err := p.ensureClient()
	if err != nil {
		return err
	}
	spec, err := torrentclient.TorrentSpecFromMetaInfoErr(meta)
	if err != nil {
		return fmt.Errorf("create torrent spec: %w", err)
	}
	torrent, _, err := client.AddTorrentSpec(spec)
	if err != nil {
		return fmt.Errorf("attach torrent: %w", err)
	}
	select {
	case <-torrent.GotInfo():
	case <-ctx.Done():
		return ctx.Err()
	}
	for _, file := range torrent.Files() {
		if file.Path() != record.LogicalPath {
			continue
		}
		p.downloadMu.Lock()
		if p.activeDownloads[record.Download.ID] == nil {
			p.beginTrackDownloadLocked(record, file)
		}
		p.downloadMu.Unlock()
		return nil
	}
	return fmt.Errorf("torrent track %q is missing", record.LogicalPath)
}

func (p *Provider) markDownloadFailed(id string, failure error) {
	p.downloadMu.Lock()
	defer p.downloadMu.Unlock()
	record, ok := p.downloads[id]
	if !ok {
		return
	}
	record.Download.State = domain.DownloadStateFailed
	record.Download.Error = failure.Error()
	record.Download.UpdatedAt = time.Now().UTC()
	p.downloads[id] = record
	_ = p.persistDownloadLocked(record)
}

func (p *Provider) markDownloadEvicted(sourceKey string) {
	id := domain.StableID("download", Name, sourceKey)
	p.downloadMu.Lock()
	defer p.downloadMu.Unlock()
	record, ok := p.downloads[id]
	if !ok || p.activeDownloads[id] != nil {
		return
	}
	record.Download.State = domain.DownloadStateEvicted
	record.Download.CompletedBytes = 0
	record.Download.UpdatedAt = time.Now().UTC()
	p.downloads[id] = record
	p.completionMu.Lock()
	delete(p.completed, record.Source.Key)
	p.completionMu.Unlock()
	_ = p.persistDownloadLocked(record)
}

func (p *Provider) removeDownloadsForSource(infoHash string) error {
	p.downloadMu.Lock()
	defer p.downloadMu.Unlock()
	for id, record := range p.downloads {
		if record.Download.SourceID != infoHash {
			continue
		}
		delete(p.downloads, id)
		delete(p.activeDownloads, id)
		if err := os.Remove(p.downloadRecordPath(id)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove torrent download job: %w", err)
		}
	}
	return nil
}

func (p *Provider) readDownloadRecord(recordPath string) (downloadRecord, error) {
	contents, err := os.ReadFile(recordPath)
	if err != nil {
		return downloadRecord{}, fmt.Errorf("read torrent download job %q: %w", filepath.Base(recordPath), err)
	}
	var record downloadRecord
	if err := json.Unmarshal(contents, &record); err != nil {
		return downloadRecord{}, fmt.Errorf("decode torrent download job %q: %w", filepath.Base(recordPath), err)
	}
	infoHash, logicalPath, err := parseSourceKey(record.Source.Key)
	expectedID := domain.StableID("download", record.Source.Provider, record.Source.Key)
	if err != nil || infoHash != record.Download.SourceID || logicalPath != record.LogicalPath ||
		record.Source.Provider != Name || record.Download.ID != expectedID || record.Download.Provider != Name {
		return downloadRecord{}, fmt.Errorf("invalid torrent download job %q", filepath.Base(recordPath))
	}
	if record.Download.TrackID == "" {
		record.Download.TrackID = domain.StableID("track", Name, infoHash, logicalPath)
	}
	return record, nil
}

func (p *Provider) persistDownloadLocked(record downloadRecord) error {
	if p.metadataRoot == "" {
		return nil
	}
	if err := os.MkdirAll(p.downloadDirectory(), 0o700); err != nil {
		return fmt.Errorf("create torrent download directory: %w", err)
	}
	contents, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode torrent download job: %w", err)
	}
	recordPath := p.downloadRecordPath(record.Download.ID)
	temporaryPath := recordPath + ".tmp"
	if err := os.WriteFile(temporaryPath, contents, 0o600); err != nil {
		return fmt.Errorf("write torrent download job: %w", err)
	}
	if err := os.Rename(temporaryPath, recordPath); err != nil {
		return fmt.Errorf("commit torrent download job: %w", err)
	}
	return nil
}

func (p *Provider) downloadDirectory() string {
	return filepath.Join(p.metadataRoot, "downloads")
}

func (p *Provider) downloadRecordPath(id string) string {
	return filepath.Join(p.downloadDirectory(), strings.TrimSpace(id)+".json")
}
