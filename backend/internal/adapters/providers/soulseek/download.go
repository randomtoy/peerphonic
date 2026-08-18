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

	mu   sync.Mutex
	jobs map[string]*downloadJob
}

type downloadJob struct {
	finalPath      string
	incompletePath string
	done           chan struct{}

	mu  sync.Mutex
	err error
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
		jobs: make(map[string]*downloadJob),
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
	job := &downloadJob{
		finalPath: filepath.Join(c.downloadsDir, trackID, name),
		incompletePath: filepath.Join(
			c.incompleteDir, sanitizedPathSegment(remote.Peer),
			sanitizedRemoteDirectory(remote.Path), name,
		),
		done: make(chan struct{}),
	}
	c.jobs[key] = job
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
		c.removeJob(key, job)
		return nil, fmt.Errorf("enqueue Soulseek download: %w", err)
	}
	if response.Batch.ID == "" || len(response.Batch.Transfers) == 0 {
		c.removeJob(key, job)
		message := "slskd did not enqueue the file"
		if len(response.Failures) > 0 && strings.TrimSpace(response.Failures[0].Message) != "" {
			message = response.Failures[0].Message
		}
		return nil, fmt.Errorf("enqueue Soulseek download: %s", message)
	}
	go c.monitorDownload(key, job, response.Batch.ID)
	return job, nil
}

func (c *downloadCoordinator) monitorDownload(key string, job *downloadJob, batchID string) {
	ticker := time.NewTicker(downloadPollInterval)
	defer ticker.Stop()
	consecutiveErrors := 0
	for {
		var batch slskdDownloadBatch
		ctx, cancel := context.WithTimeout(context.Background(), c.client.httpClient.Timeout)
		err := c.client.doJSON(ctx, http.MethodGet,
			"/api/v0/transfers/downloads/batches/"+url.PathEscape(batchID), nil, &batch)
		cancel()
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= 5 {
				c.finishJob(key, job, fmt.Errorf("monitor Soulseek download: %w", err))
				return
			}
		} else {
			consecutiveErrors = 0
			if len(batch.Transfers) == 0 {
				c.finishJob(key, job, errors.New("monitor Soulseek download: batch has no transfers"))
				return
			}
			transfer := batch.Transfers[0]
			if transferSucceeded(transfer.State) {
				if err := waitForFinalFile(job.finalPath, 10*time.Second); err != nil {
					c.finishJob(key, job, err)
					return
				}
				c.finishJob(key, job, nil)
				return
			}
			if transferFailed(transfer.State) {
				message := strings.TrimSpace(transfer.Exception)
				if message == "" {
					message = transfer.State
				}
				c.finishJob(key, job, fmt.Errorf("Soulseek download failed: %s", message))
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

func (c *downloadCoordinator) finishJob(key string, job *downloadJob, err error) {
	job.mu.Lock()
	job.err = err
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
