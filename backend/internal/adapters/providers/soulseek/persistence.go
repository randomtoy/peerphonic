package soulseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func (c *downloadCoordinator) persistJob(job *downloadJob) error {
	c.persistMu.Lock()
	defer c.persistMu.Unlock()
	record := job.persisted()
	contents, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode Soulseek download job: %w", err)
	}
	recordPath := c.recordPath(record.Download.ID)
	temporaryPath := recordPath + ".tmp"
	if err := os.WriteFile(temporaryPath, contents, 0o600); err != nil {
		return fmt.Errorf("write Soulseek download job: %w", err)
	}
	if err := os.Rename(temporaryPath, recordPath); err != nil {
		return fmt.Errorf("commit Soulseek download job: %w", err)
	}
	job.mu.Lock()
	job.lastPersist = time.Now()
	job.mu.Unlock()
	return nil
}

func (j *downloadJob) persisted() persistedDownloadJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	return persistedDownloadJob{
		Download: j.download, Key: j.key, TrackID: j.trackID, Remote: j.remote,
		BatchID: j.batchID, TransferID: j.transferID,
	}
}

func (c *downloadCoordinator) loadRecords() []error {
	entries, err := os.ReadDir(c.stateDir)
	if err != nil {
		return []error{fmt.Errorf("read Soulseek download jobs: %w", err)}
	}
	var loadErrors []error
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		recordPath := filepath.Join(c.stateDir, entry.Name())
		record, err := readPersistedJob(recordPath)
		if err != nil {
			loadErrors = append(loadErrors, err)
			continue
		}
		job, err := c.restoreJob(record)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("restore Soulseek download job %q: %w", entry.Name(), err))
			continue
		}
		c.records[record.Download.ID] = job
		if !job.finished {
			c.jobs[record.Key] = job
		}
		if err := c.persistJob(job); err != nil {
			loadErrors = append(loadErrors, err)
		}
	}
	return loadErrors
}

func readPersistedJob(path string) (persistedDownloadJob, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return persistedDownloadJob{}, fmt.Errorf("read Soulseek download job %q: %w", filepath.Base(path), err)
	}
	var record persistedDownloadJob
	if err := json.Unmarshal(contents, &record); err != nil {
		return persistedDownloadJob{}, fmt.Errorf("decode Soulseek download job %q: %w", filepath.Base(path), err)
	}
	expectedDownloadID := domain.StableID("download", Name, record.Key)
	if record.Download.ID != expectedDownloadID || record.Download.Provider != Name ||
		record.Download.TrackID != record.TrackID || strings.TrimSpace(record.TrackID) == "" ||
		record.Remote.Peer == "" || record.Remote.Path == "" || record.Remote.Size <= 0 ||
		strings.TrimSpace(record.Key) == "" {
		return persistedDownloadJob{}, fmt.Errorf("invalid Soulseek download job %q", filepath.Base(path))
	}
	decoded, err := decodeRemoteFileRef(record.Key)
	if err != nil || decoded != record.Remote {
		return persistedDownloadJob{}, fmt.Errorf("invalid Soulseek source in job %q", filepath.Base(path))
	}
	return record, nil
}

func (c *downloadCoordinator) restoreJob(record persistedDownloadJob) (*downloadJob, error) {
	name := sanitizedFilename(record.Remote.Path)
	job := &downloadJob{
		key: record.Key, trackID: record.TrackID, remote: record.Remote,
		batchID: record.BatchID, transferID: record.TransferID,
		finalPath: filepath.Join(c.downloadsDir, record.TrackID, name),
		incompletePath: filepath.Join(
			c.incompleteDir, sanitizedPathSegment(record.Remote.Peer),
			sanitizedRemoteDirectory(record.Remote.Path), name,
		),
		done: make(chan struct{}), ready: make(chan struct{}), download: record.Download,
	}
	job.markEnqueued()
	if info, err := os.Stat(job.finalPath); err == nil {
		if info.Size() != record.Remote.Size {
			job.download.State = domain.DownloadStateEvicted
			job.download.CompletedBytes = 0
			job.download.Error = ""
			job.finished = true
			close(job.done)
			return job, nil
		}
		job.download.State = domain.DownloadStateCached
		job.download.CompletedBytes = record.Remote.Size
		job.download.Error = ""
		job.finished = true
		close(job.done)
		return job, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect cached file: %w", err)
	}
	if record.Download.State == domain.DownloadStateQueued || record.Download.State == domain.DownloadStateDownloading {
		if record.BatchID == "" || record.TransferID == "" {
			job.download.State = domain.DownloadStateFailed
			job.download.Error = "active Soulseek job is missing slskd transfer identifiers"
			job.finished = true
			close(job.done)
			return job, nil
		}
		return job, nil
	}
	if record.Download.State == domain.DownloadStateCached {
		job.download.State = domain.DownloadStateEvicted
		job.download.CompletedBytes = 0
	}
	job.finished = true
	close(job.done)
	return job, nil
}

func (c *downloadCoordinator) recordPath(id string) string {
	return filepath.Join(c.stateDir, strings.TrimSpace(id)+".json")
}
