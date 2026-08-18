package soulseek

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func (c *Client) ResumeTrackDownloads(ctx context.Context) error {
	if c.downloads == nil {
		return nil
	}
	return c.downloads.resume(ctx)
}

func (c *downloadCoordinator) resume(ctx context.Context) error {
	var resumeErrors []error
	resumeErrors = append(resumeErrors, c.loadErrors...)
	c.mu.Lock()
	jobs := make([]*downloadJob, 0, len(c.jobs))
	for _, job := range c.jobs {
		jobs = append(jobs, job)
	}
	c.mu.Unlock()
	c.mu.Lock()
	records := make([]*downloadJob, 0, len(c.records))
	for _, job := range c.records {
		records = append(records, job)
	}
	c.mu.Unlock()
	for _, job := range records {
		download, _ := job.snapshot()
		if download.State == domain.DownloadStateCached {
			c.client.notifyCompleted(CompletedFile{TrackID: download.TrackID, Path: job.finalPath})
		}
	}
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(resumeErrors, err)...)
		}
		job.mu.Lock()
		key, batchID := job.key, job.batchID
		job.mu.Unlock()
		if batchID == "" {
			err := errors.New("resume Soulseek download: missing slskd batch ID")
			c.finishJob(key, job, domain.DownloadStateFailed, err)
			resumeErrors = append(resumeErrors, err)
			continue
		}
		c.startMonitor(key, job, batchID)
	}
	return errors.Join(resumeErrors...)
}

func (c *Client) TrackDownloads(ctx context.Context) ([]domain.TrackDownload, error) {
	if c.downloads == nil {
		return nil, nil
	}
	return c.downloads.trackDownloads(ctx)
}

func (c *downloadCoordinator) trackDownloads(ctx context.Context) ([]domain.TrackDownload, error) {
	c.mu.Lock()
	jobs := make([]*downloadJob, 0, len(c.records))
	for _, job := range c.records {
		jobs = append(jobs, job)
	}
	c.mu.Unlock()

	result := make([]domain.TrackDownload, 0, len(jobs))
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		download, changed := job.snapshot()
		if changed {
			if err := c.persistJob(job); err != nil {
				return nil, err
			}
		}
		result = append(result, download)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].Name < result[j].Name
		}
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result, nil
}

func (j *downloadJob) snapshot() (domain.TrackDownload, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	changed := false
	if j.download.State == domain.DownloadStateCached {
		if _, err := os.Stat(j.finalPath); errors.Is(err, os.ErrNotExist) {
			j.download.State = domain.DownloadStateEvicted
			j.download.CompletedBytes = 0
			j.download.UpdatedAt = time.Now().UTC()
			changed = true
		}
	}
	return j.download, changed
}

func (c *Client) CancelTrackDownload(ctx context.Context, id string) error {
	if c.downloads == nil {
		return ports.ErrNotFound
	}
	return c.downloads.cancel(ctx, id)
}

func (c *downloadCoordinator) cancel(ctx context.Context, id string) error {
	c.mu.Lock()
	job := c.records[id]
	c.mu.Unlock()
	if job == nil {
		return ports.ErrNotFound
	}
	job.mu.Lock()
	state := job.download.State
	peer := job.remote.Peer
	transferID := job.transferID
	key := job.key
	job.mu.Unlock()
	if state != domain.DownloadStateQueued && state != domain.DownloadStateDownloading {
		return fmt.Errorf("download %q is not active", id)
	}
	if peer == "" || transferID == "" {
		return fmt.Errorf("download %q has not been enqueued by slskd", id)
	}
	path := "/api/v0/transfers/downloads/" + url.PathEscape(peer) + "/" + url.PathEscape(transferID)
	if err := c.client.doJSON(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("cancel slskd download: %w", err)
	}
	c.finishJob(key, job, domain.DownloadStateCancelled, errors.New("Soulseek download cancelled"))
	return nil
}

func (c *Client) RetryTrackDownload(ctx context.Context, id string) error {
	if c.downloads == nil {
		return ports.ErrNotFound
	}
	return c.downloads.retry(ctx, id)
}

func (c *downloadCoordinator) retry(ctx context.Context, id string) error {
	c.mu.Lock()
	job := c.records[id]
	c.mu.Unlock()
	if job == nil {
		return ports.ErrNotFound
	}
	job.mu.Lock()
	state := job.download.State
	key, trackID, remote := job.key, job.trackID, job.remote
	name := job.download.Name
	job.mu.Unlock()
	if state != domain.DownloadStateFailed && state != domain.DownloadStateCancelled && state != domain.DownloadStateEvicted {
		return fmt.Errorf("download %q is not retryable", id)
	}
	_, err := c.ensureDownload(ctx, key, trackID, name, remote)
	return err
}

func (c *Client) CacheUsage(ctx context.Context) (domain.CacheUsage, error) {
	usage := domain.CacheUsage{Name: Name}
	if c.downloads == nil {
		return usage, nil
	}
	for _, root := range []struct {
		path    string
		partial bool
	}{
		{path: c.downloads.downloadsDir},
		{path: c.downloads.incompleteDir, partial: true},
	} {
		err := filepath.WalkDir(root.path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			usage.Entries++
			usage.Size += info.Size()
			if root.partial {
				usage.PartialEntries++
			}
			return nil
		})
		if err != nil {
			return domain.CacheUsage{}, fmt.Errorf("inspect Soulseek cache %q: %w", root.path, err)
		}
	}
	return usage, nil
}
