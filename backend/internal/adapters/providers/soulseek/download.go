package soulseek

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const downloadPollInterval = 500 * time.Millisecond

var (
	driveRootPattern      = regexp.MustCompile(`^[a-zA-Z]:[/\\]?`)
	uncRootPattern        = regexp.MustCompile(`^[/\\]{2}[^/\\]+[/\\]?`)
	soulseekQtRootPattern = regexp.MustCompile(`^@@[a-zA-Z0-9]{5,}[/\\]?`)
)

type remoteFileRef struct {
	Peer string `json:"peer"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type downloadCoordinator struct {
	client        *Client
	downloadsDir  string
	incompleteDir string

	mu      sync.Mutex
	jobs    map[string]*downloadJob
	records map[string]*downloadJob
}

type downloadJob struct {
	key            string
	trackID        string
	remote         remoteFileRef
	batchID        string
	transferID     string
	finalPath      string
	incompletePath string
	done           chan struct{}

	mu       sync.Mutex
	download domain.TrackDownload
	err      error
	finished bool
}

type slskdDownloadBatchResponse struct {
	Batch    slskdDownloadBatch  `json:"batch"`
	Failures []slskdBatchFailure `json:"failures"`
}

type slskdDownloadBatch struct {
	ID        string                  `json:"id"`
	Transfers []slskdDownloadTransfer `json:"transfers"`
}

type slskdDownloadTransfer struct {
	ID               string `json:"id"`
	Filename         string `json:"filename"`
	State            string `json:"state"`
	BytesTransferred int64  `json:"bytesTransferred"`
	Exception        string `json:"exception"`
}

type slskdBatchFailure struct {
	Filename string `json:"filename"`
	Message  string `json:"message"`
}

func newDownloadCoordinator(
	client *Client, downloadsDir, incompleteDir string,
) (*downloadCoordinator, error) {
	downloadsDir = strings.TrimSpace(downloadsDir)
	incompleteDir = strings.TrimSpace(incompleteDir)
	if downloadsDir == "" || incompleteDir == "" {
		return nil, errors.New("slskd downloads and incomplete directories are required")
	}
	var err error
	downloadsDir, err = filepath.Abs(downloadsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve slskd downloads directory: %w", err)
	}
	incompleteDir, err = filepath.Abs(incompleteDir)
	if err != nil {
		return nil, fmt.Errorf("resolve slskd incomplete directory: %w", err)
	}
	for _, directory := range []string{downloadsDir, incompleteDir} {
		if err := os.MkdirAll(directory, 0o750); err != nil {
			return nil, fmt.Errorf("create slskd media directory %q: %w", directory, err)
		}
	}
	return &downloadCoordinator{
		client: client, downloadsDir: downloadsDir, incompleteDir: incompleteDir,
		jobs: make(map[string]*downloadJob), records: make(map[string]*downloadJob),
	}, nil
}

func (c *Client) Resolve(ctx context.Context, ref domain.SourceRef) (ports.ResolvedSource, error) {
	if ref.Provider != Name {
		return ports.ResolvedSource{}, fmt.Errorf("unexpected provider %q", ref.Provider)
	}
	if c.downloads == nil {
		return ports.ResolvedSource{}, fmt.Errorf("%w: slskd media directories are not configured", ports.ErrSourceUnavailable)
	}
	remote, err := decodeRemoteFileRef(ref.Key)
	if err != nil {
		return ports.ResolvedSource{}, err
	}
	return c.downloads.resolve(ctx, ref.Key, remote)
}

func decodeRemoteFileRef(key string) (remoteFileRef, error) {
	encoded, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil {
		return remoteFileRef{}, fmt.Errorf("decode Soulseek source reference: %w", err)
	}
	var remote remoteFileRef
	if err := json.Unmarshal(encoded, &remote); err != nil {
		return remoteFileRef{}, fmt.Errorf("decode Soulseek source reference: %w", err)
	}
	remote.Peer = strings.TrimSpace(remote.Peer)
	remote.Path = strings.TrimSpace(remote.Path)
	if remote.Peer == "" || remote.Path == "" || remote.Size <= 0 {
		return remoteFileRef{}, errors.New("invalid Soulseek source reference")
	}
	return remote, nil
}

func (c *downloadCoordinator) resolve(
	ctx context.Context, key string, remote remoteFileRef,
) (ports.ResolvedSource, error) {
	trackID := domain.StableID(Name, remote.Peer, remote.Path, strconv.FormatInt(remote.Size, 10))
	name := sanitizedFilename(remote.Path)
	finalPath := filepath.Join(c.downloadsDir, trackID, name)
	if info, err := os.Stat(finalPath); err == nil {
		if info.Size() != remote.Size {
			return ports.ResolvedSource{}, fmt.Errorf("cached Soulseek file has size %d, want %d", info.Size(), remote.Size)
		}
		file, err := os.Open(finalPath)
		if err != nil {
			return ports.ResolvedSource{}, fmt.Errorf("open cached Soulseek file: %w", err)
		}
		c.rememberCached(key, trackID, name, remote, info)
		return resolvedRemoteSource(file, name, remote.Size, info.ModTime()), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return ports.ResolvedSource{}, fmt.Errorf("inspect cached Soulseek file: %w", err)
	}

	job, err := c.ensureDownload(ctx, key, trackID, name, remote)
	if err != nil {
		return ports.ResolvedSource{}, err
	}
	reader := newGrowingFile(job, remote.Size)
	return resolvedRemoteSource(reader, name, remote.Size, time.Now()), nil
}

func resolvedRemoteSource(
	content ports.ReadSeekCloser, name string, size int64, modTime time.Time,
) ports.ResolvedSource {
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	return ports.ResolvedSource{
		Content: content, Name: name, ContentType: audioContentTypes[extension],
		Size: size, ModTime: modTime,
	}
}

func (c *downloadCoordinator) ensureDownload(
	ctx context.Context, key, trackID, name string, remote remoteFileRef,
) (*downloadJob, error) {
	c.mu.Lock()
	if job := c.jobs[key]; job != nil {
		c.mu.Unlock()
		return job, nil
	}
	now := time.Now().UTC()
	downloadID := domain.StableID("download", Name, key)
	job := &downloadJob{
		key: key, trackID: trackID, remote: remote,
		finalPath: filepath.Join(c.downloadsDir, trackID, name),
		incompletePath: filepath.Join(
			c.incompleteDir, sanitizedPathSegment(remote.Peer),
			sanitizedRemoteDirectory(remote.Path), name,
		),
		done: make(chan struct{}),
		download: domain.TrackDownload{
			ID: downloadID, Provider: Name, SourceID: remote.Peer, TrackID: trackID,
			Name: name, State: domain.DownloadStateQueued, TotalBytes: remote.Size,
			StartedAt: now, UpdatedAt: now,
		},
	}
	c.jobs[key] = job
	c.records[downloadID] = job
	c.mu.Unlock()

	payload := struct {
		Username string `json:"username"`
		Files    []struct {
			Filename string `json:"filename"`
			Size     int64  `json:"size"`
		} `json:"files"`
		Options struct {
			Destination string `json:"destination"`
			ExternalID  string `json:"externalId"`
		} `json:"options"`
	}{Username: remote.Peer}
	payload.Files = append(payload.Files, struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	}{Filename: remote.Path, Size: remote.Size})
	payload.Options.Destination = trackID
	payload.Options.ExternalID = trackID

	var response slskdDownloadBatchResponse
	if err := c.client.doJSON(ctx, http.MethodPost, "/api/v0/transfers/downloads/batches", payload, &response); err != nil {
		err = fmt.Errorf("enqueue Soulseek download: %w", err)
		c.finishJob(key, job, domain.DownloadStateFailed, err)
		return nil, err
	}
	if response.Batch.ID == "" || len(response.Batch.Transfers) == 0 {
		message := "slskd did not enqueue the file"
		if len(response.Failures) > 0 && strings.TrimSpace(response.Failures[0].Message) != "" {
			message = response.Failures[0].Message
		}
		err := fmt.Errorf("enqueue Soulseek download: %s", message)
		c.finishJob(key, job, domain.DownloadStateFailed, err)
		return nil, err
	}
	job.mu.Lock()
	job.batchID = response.Batch.ID
	job.transferID = response.Batch.Transfers[0].ID
	job.mu.Unlock()
	go c.monitorDownload(key, job, response.Batch.ID)
	return job, nil
}

func (c *downloadCoordinator) monitorDownload(key string, job *downloadJob, batchID string) {
	ticker := time.NewTicker(downloadPollInterval)
	defer ticker.Stop()
	consecutiveErrors := 0
	for {
		if job.isFinished() {
			return
		}
		var batch slskdDownloadBatch
		ctx, cancel := context.WithTimeout(context.Background(), c.client.httpClient.Timeout)
		err := c.client.doJSON(ctx, http.MethodGet,
			"/api/v0/transfers/downloads/batches/"+url.PathEscape(batchID), nil, &batch)
		cancel()
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= 5 {
				c.finishJob(key, job, domain.DownloadStateFailed, fmt.Errorf("monitor Soulseek download: %w", err))
				return
			}
		} else {
			consecutiveErrors = 0
			if len(batch.Transfers) == 0 {
				c.finishJob(key, job, domain.DownloadStateFailed, errors.New("monitor Soulseek download: batch has no transfers"))
				return
			}
			transfer := batch.Transfers[0]
			job.updateTransfer(transfer)
			if transferSucceeded(transfer.State) {
				if err := waitForFinalFile(job.finalPath, 10*time.Second); err != nil {
					c.finishJob(key, job, domain.DownloadStateFailed, err)
					return
				}
				c.finishJob(key, job, domain.DownloadStateCached, nil)
				return
			}
			if transferFailed(transfer.State) {
				message := strings.TrimSpace(transfer.Exception)
				if message == "" {
					message = transfer.State
				}
				state := domain.DownloadStateFailed
				if stateContains(transfer.State, "Cancelled") {
					state = domain.DownloadStateCancelled
				}
				c.finishJob(key, job, state, fmt.Errorf("Soulseek download failed: %s", message))
				return
			}
		}
		<-ticker.C
	}
}

func transferSucceeded(state string) bool {
	return stateContains(state, "Completed") && stateContains(state, "Succeeded")
}

func transferFailed(state string) bool {
	if !stateContains(state, "Completed") {
		return false
	}
	for _, terminal := range []string{"Errored", "Rejected", "Cancelled", "TimedOut", "Aborted"} {
		if stateContains(state, terminal) {
			return true
		}
	}
	return false
}

func stateContains(state, flag string) bool {
	for _, value := range strings.FieldsFunc(state, func(r rune) bool { return r == ',' || r == '|' }) {
		if strings.EqualFold(strings.TrimSpace(value), flag) {
			return true
		}
	}
	return false
}

func waitForFinalFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect completed Soulseek download: %w", err)
		}
		if time.Now().After(deadline) {
			return errors.New("completed Soulseek download did not appear in the shared downloads directory")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (c *downloadCoordinator) finishJob(
	key string, job *downloadJob, state domain.DownloadState, err error,
) {
	job.mu.Lock()
	if job.finished {
		job.mu.Unlock()
		return
	}
	job.finished = true
	job.err = err
	job.download.State = state
	job.download.UpdatedAt = time.Now().UTC()
	if state == domain.DownloadStateCached {
		job.download.CompletedBytes = job.download.TotalBytes
		job.download.Error = ""
	} else if err != nil {
		job.download.Error = err.Error()
	}
	close(job.done)
	job.mu.Unlock()
	c.removeJob(key, job)
}

func (c *downloadCoordinator) removeJob(key string, job *downloadJob) {
	c.mu.Lock()
	if c.jobs[key] == job {
		delete(c.jobs, key)
	}
	c.mu.Unlock()
}

func (j *downloadJob) result() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.err
}

func (j *downloadJob) isFinished() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.finished
}

func (j *downloadJob) updateTransfer(transfer slskdDownloadTransfer) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.finished {
		return
	}
	if transfer.ID != "" {
		j.transferID = transfer.ID
	}
	j.download.CompletedBytes = min(transfer.BytesTransferred, j.download.TotalBytes)
	j.download.UpdatedAt = time.Now().UTC()
	if stateContains(transfer.State, "InProgress") {
		j.download.State = domain.DownloadStateDownloading
	} else if stateContains(transfer.State, "Queued") || stateContains(transfer.State, "Requested") ||
		stateContains(transfer.State, "Initializing") {
		j.download.State = domain.DownloadStateQueued
	}
}

func (c *downloadCoordinator) rememberCached(
	key, trackID, name string, remote remoteFileRef, info os.FileInfo,
) {
	id := domain.StableID("download", Name, key)
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.records[id]; existing != nil {
		existing.mu.Lock()
		existing.download.State = domain.DownloadStateCached
		existing.download.CompletedBytes = remote.Size
		existing.download.TotalBytes = remote.Size
		existing.download.Error = ""
		existing.download.UpdatedAt = info.ModTime().UTC()
		existing.mu.Unlock()
		return
	}
	completedAt := info.ModTime().UTC()
	c.records[id] = &downloadJob{
		key: key, trackID: trackID, remote: remote,
		finalPath: filepath.Join(c.downloadsDir, trackID, name),
		download: domain.TrackDownload{
			ID: id, Provider: Name, SourceID: remote.Peer, TrackID: trackID,
			Name: name, State: domain.DownloadStateCached,
			CompletedBytes: remote.Size, TotalBytes: remote.Size,
			StartedAt: completedAt, UpdatedAt: completedAt,
		},
		finished: true,
	}
}

func sanitizedFilename(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return "audio"
	}
	return sanitizedPathSegment(parts[len(parts)-1])
}

func sanitizedRemoteDirectory(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = driveRootPattern.ReplaceAllString(path, "")
	path = uncRootPattern.ReplaceAllString(path, "")
	path = soulseekQtRootPattern.ReplaceAllString(path, "")
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) <= 1 {
		return ""
	}
	for index := range parts[:len(parts)-1] {
		parts[index] = sanitizedPathSegment(parts[index])
	}
	return filepath.Join(parts[:len(parts)-1]...)
}

func sanitizedPathSegment(value string) string {
	value = strings.NewReplacer("\x00", "_", "/", "_", "\\", "_").Replace(value)
	if value == "." || value == ".." {
		return "_"
	}
	return value
}
