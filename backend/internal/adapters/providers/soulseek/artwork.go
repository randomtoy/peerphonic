package soulseek

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const (
	remoteArtworkPrefix  = "soulseek_art_"
	maxRemoteArtworkSize = 32 << 20
)

type artworkDownload struct {
	done      chan struct{}
	finalPath string

	mu       sync.Mutex
	err      error
	finished bool
}

func (c *Client) OpenArtwork(ctx context.Context, id string) (ports.ResolvedSource, error) {
	if !strings.HasPrefix(id, remoteArtworkPrefix) {
		return ports.ResolvedSource{}, ports.ErrNotFound
	}
	if c.downloads == nil {
		return ports.ResolvedSource{}, fmt.Errorf("%w: slskd media directories are not configured", ports.ErrSourceUnavailable)
	}
	key := strings.TrimPrefix(id, remoteArtworkPrefix)
	remote, err := decodeRemoteFileRef(key)
	if err != nil {
		return ports.ResolvedSource{}, ports.ErrNotFound
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(remote.Path)), ".")
	if _, ok := artworkExtensions[extension]; !ok || remote.Size > maxRemoteArtworkSize {
		return ports.ResolvedSource{}, ports.ErrNotFound
	}
	destination := domain.StableID("soulseek-cover", key)
	name := sanitizedFilename(remote.Path)
	finalPath := filepath.Join(c.downloads.downloadsDir, destination, name)
	if resolved, err := openRemoteArtwork(finalPath, name, remote.Size); err == nil {
		return resolved, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return ports.ResolvedSource{}, err
	}

	job, err := c.ensureArtworkDownload(ctx, destination, finalPath, remote)
	if err != nil {
		return ports.ResolvedSource{}, err
	}
	select {
	case <-ctx.Done():
		return ports.ResolvedSource{}, ctx.Err()
	case <-job.done:
		if err := job.result(); err != nil {
			return ports.ResolvedSource{}, err
		}
	}
	return openRemoteArtwork(finalPath, name, remote.Size)
}

func openRemoteArtwork(path, name string, size int64) (ports.ResolvedSource, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ports.ResolvedSource{}, err
	}
	if info.Size() != size {
		return ports.ResolvedSource{}, fmt.Errorf("cached Soulseek artwork has size %d, want %d", info.Size(), size)
	}
	file, err := os.Open(path)
	if err != nil {
		return ports.ResolvedSource{}, fmt.Errorf("open cached Soulseek artwork: %w", err)
	}
	contentType := "image/" + strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if strings.HasSuffix(strings.ToLower(name), ".jpg") {
		contentType = "image/jpeg"
	}
	return ports.ResolvedSource{
		Content: file, Name: name, ContentType: contentType, Size: size, ModTime: info.ModTime(),
	}, nil
}

func (c *Client) ensureArtworkDownload(
	ctx context.Context, destination, finalPath string, remote remoteFileRef,
) (*artworkDownload, error) {
	c.artworkMu.Lock()
	if c.artworkJobs == nil {
		c.artworkJobs = make(map[string]*artworkDownload)
	}
	if job := c.artworkJobs[destination]; job != nil {
		c.artworkMu.Unlock()
		return job, nil
	}
	job := &artworkDownload{done: make(chan struct{}), finalPath: finalPath}
	c.artworkJobs[destination] = job
	c.artworkMu.Unlock()

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
	payload.Options.Destination = destination
	payload.Options.ExternalID = destination

	var response slskdDownloadBatchResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v0/transfers/downloads/batches", payload, &response); err != nil {
		err = fmt.Errorf("enqueue Soulseek artwork: %w", err)
		c.finishArtworkDownload(destination, job, err)
		return nil, err
	}
	if response.Batch.ID == "" || len(response.Batch.Transfers) == 0 {
		message := "slskd did not enqueue the artwork"
		if len(response.Failures) > 0 && strings.TrimSpace(response.Failures[0].Message) != "" {
			message = response.Failures[0].Message
		}
		err := errors.New(message)
		c.finishArtworkDownload(destination, job, err)
		return nil, err
	}
	c.artworkWG.Add(1)
	go func() {
		defer c.artworkWG.Done()
		c.monitorArtworkDownload(destination, job, response.Batch.ID)
	}()
	return job, nil
}

func (c *Client) monitorArtworkDownload(destination string, job *artworkDownload, batchID string) {
	ticker := time.NewTicker(downloadPollInterval)
	defer ticker.Stop()
	consecutiveErrors := 0
	for {
		var batch slskdDownloadBatch
		ctx, cancel := context.WithTimeout(c.downloads.ctx, c.httpClient.Timeout)
		err := c.doJSON(ctx, http.MethodGet,
			"/api/v0/transfers/downloads/batches/"+url.PathEscape(batchID), nil, &batch)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				c.finishArtworkDownload(destination, job, err)
				return
			}
			consecutiveErrors++
			if consecutiveErrors >= 5 {
				c.finishArtworkDownload(destination, job, fmt.Errorf("monitor Soulseek artwork: %w", err))
				return
			}
		} else {
			consecutiveErrors = 0
			if len(batch.Transfers) == 0 {
				c.finishArtworkDownload(destination, job, errors.New("monitor Soulseek artwork: batch has no transfers"))
				return
			}
			transfer := batch.Transfers[0]
			if transferSucceeded(transfer.State) {
				if err := waitForFinalFile(job.finalPath, 10*time.Second); err != nil {
					c.finishArtworkDownload(destination, job, err)
					return
				}
				c.finishArtworkDownload(destination, job, nil)
				return
			}
			if transferFailed(transfer.State) {
				message := strings.TrimSpace(transfer.Exception)
				if message == "" {
					message = transfer.State
				}
				c.finishArtworkDownload(destination, job, fmt.Errorf("Soulseek artwork download failed: %s", message))
				return
			}
		}
		select {
		case <-c.downloads.ctx.Done():
			c.finishArtworkDownload(destination, job, c.downloads.ctx.Err())
			return
		case <-ticker.C:
		}
	}
}

func (c *Client) finishArtworkDownload(destination string, job *artworkDownload, err error) {
	job.mu.Lock()
	if job.finished {
		job.mu.Unlock()
		return
	}
	job.err = err
	job.finished = true
	close(job.done)
	job.mu.Unlock()
	if err != nil {
		c.artworkMu.Lock()
		if c.artworkJobs[destination] == job {
			delete(c.artworkJobs, destination)
		}
		c.artworkMu.Unlock()
	}
}

func (j *artworkDownload) result() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.err
}
