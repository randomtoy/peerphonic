package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const defaultCachedMP3BitRate = 192

type CachedAudioTranscoder struct {
	delegate ports.AudioTranscoder
	cache    *MediaCache
	now      func() time.Time

	mu      sync.Mutex
	flights map[string]*transcodeFlight
	jobs    map[string]domain.TrackDownload
}

type transcodeFlight struct {
	done chan struct{}
	err  error
}

func NewCachedAudioTranscoder(delegate ports.AudioTranscoder, cache *MediaCache) *CachedAudioTranscoder {
	return &CachedAudioTranscoder{
		delegate: delegate, cache: cache, now: time.Now,
		flights: make(map[string]*transcodeFlight), jobs: make(map[string]domain.TrackDownload),
	}
}

func (c *CachedAudioTranscoder) Transcode(
	ctx context.Context, source ports.ResolvedSource, options ports.AudioTranscodeOptions,
) (ports.TranscodedSource, error) {
	options.Format = strings.ToLower(strings.TrimSpace(options.Format))
	if options.BitRate == 0 && options.Format == "mp3" {
		options.BitRate = defaultCachedMP3BitRate
	}
	key := transcodeCacheKey(source, options)
	name := replaceAudioExtension(source.Name, "."+options.Format)
	if cached, err := c.openCached(ctx, key, name); err == nil {
		_ = source.Content.Close()
		c.recordCached(key, source, options, name, cached.Size)
		return cached, nil
	} else if !errors.Is(err, ports.ErrNotFound) {
		return ports.TranscodedSource{}, fmt.Errorf("open cached transcode: %w", err)
	}

	c.mu.Lock()
	if flight := c.flights[key]; flight != nil {
		c.mu.Unlock()
		_ = source.Content.Close()
		select {
		case <-flight.done:
			if flight.err != nil {
				return ports.TranscodedSource{}, flight.err
			}
			return c.openCached(ctx, key, name)
		case <-ctx.Done():
			return ports.TranscodedSource{}, fmt.Errorf("wait for cached transcode: %w", ctx.Err())
		}
	}
	flight := &transcodeFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.recordJobLocked(key, source, options, name, domain.DownloadStateTranscoding, 0, 0, "")
	c.mu.Unlock()

	transcoded, err := c.delegate.Transcode(ctx, source, options)
	if err != nil {
		c.finishFlight(key, fmt.Errorf("start audio transcode: %w", err), 0)
		return ports.TranscodedSource{}, err
	}
	reader, writer := io.Pipe()
	cacheResult := make(chan error, 1)
	go func() {
		err := c.cache.Put(context.WithoutCancel(ctx), key, reader, false)
		_ = reader.CloseWithError(err)
		cacheResult <- err
	}()
	wrapped := &cachingTranscodeReader{
		source: transcoded.Content, cacheWriter: writer, cacheResult: cacheResult,
		owner: c, key: key, lastProgress: c.now(),
	}
	transcoded.Content = wrapped
	transcoded.Name = name
	transcoded.ContentType = contentTypeForTranscode(options.Format)
	transcoded.Size = 0
	transcoded.Cached = false
	return transcoded, nil
}

func (c *CachedAudioTranscoder) TrackDownloads(ctx context.Context) ([]domain.TrackDownload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]domain.TrackDownload, 0, len(c.jobs))
	for _, job := range c.jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt.After(result[j].StartedAt) })
	return result, nil
}

func (c *CachedAudioTranscoder) openCached(
	ctx context.Context, key, name string,
) (ports.TranscodedSource, error) {
	content, err := c.cache.Open(ctx, key)
	if err != nil {
		return ports.TranscodedSource{}, err
	}
	size, err := content.Seek(0, io.SeekEnd)
	if err == nil {
		_, err = content.Seek(0, io.SeekStart)
	}
	if err != nil {
		content.Close()
		return ports.TranscodedSource{}, fmt.Errorf("inspect cached transcode: %w", err)
	}
	return ports.TranscodedSource{
		Content: content, Name: name, ContentType: contentTypeForTranscode(filepath.Ext(name)),
		Size: size, ModTime: c.now(), Cached: true,
	}, nil
}

func (c *CachedAudioTranscoder) recordCached(
	key string, source ports.ResolvedSource, options ports.AudioTranscodeOptions, name string, size int64,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordJobLocked(key, source, options, name, domain.DownloadStateCached, size, size, "")
}

func (c *CachedAudioTranscoder) recordJobLocked(
	key string, source ports.ResolvedSource, options ports.AudioTranscodeOptions, name string,
	state domain.DownloadState, completed, total int64, message string,
) {
	now := c.now().UTC()
	job := c.jobs[key]
	if job.ID == "" {
		job = domain.TrackDownload{
			ID: domain.StableID("download", "transcode", key), Provider: "transcode",
			SourceID: source.Revision, TrackID: options.TrackID, Name: name, StartedAt: now,
		}
	}
	job.State = state
	job.CompletedBytes = completed
	job.TotalBytes = total
	job.Error = message
	job.UpdatedAt = now
	c.jobs[key] = job
}

func (c *CachedAudioTranscoder) updateProgress(key string, completed int64) {
	c.mu.Lock()
	job := c.jobs[key]
	job.State = domain.DownloadStateTranscoding
	job.CompletedBytes = completed
	job.UpdatedAt = c.now().UTC()
	c.jobs[key] = job
	c.mu.Unlock()
}

func (c *CachedAudioTranscoder) finishFlight(key string, err error, size int64) {
	c.mu.Lock()
	flight := c.flights[key]
	if flight == nil {
		c.mu.Unlock()
		return
	}
	flight.err = err
	delete(c.flights, key)
	job := c.jobs[key]
	job.UpdatedAt = c.now().UTC()
	if err != nil {
		job.State = domain.DownloadStateFailed
		job.Error = err.Error()
	} else {
		job.State = domain.DownloadStateCached
		job.CompletedBytes = size
		job.TotalBytes = size
		job.Error = ""
	}
	c.jobs[key] = job
	close(flight.done)
	c.mu.Unlock()
}

type cachingTranscodeReader struct {
	source       io.ReadCloser
	cacheWriter  *io.PipeWriter
	cacheResult  <-chan error
	owner        *CachedAudioTranscoder
	key          string
	completed    int64
	lastProgress time.Time
	finished     bool
	closeOnce    sync.Once
}

func (r *cachingTranscodeReader) Read(buffer []byte) (int, error) {
	n, readErr := r.source.Read(buffer)
	if n > 0 && !r.finished {
		if _, err := r.cacheWriter.Write(buffer[:n]); err != nil {
			r.finalize(fmt.Errorf("cache transcoded audio: %w", err))
		} else {
			r.completed += int64(n)
			if now := r.owner.now(); now.Sub(r.lastProgress) >= time.Second {
				r.owner.updateProgress(r.key, r.completed)
				r.lastProgress = now
			}
		}
	}
	if readErr != nil && !r.finished {
		if errors.Is(readErr, io.EOF) {
			r.finalize(nil)
		} else {
			r.finalize(readErr)
		}
	}
	return n, readErr
}

func (r *cachingTranscodeReader) Close() error {
	var closeErr error
	r.closeOnce.Do(func() {
		closeErr = r.source.Close()
		if !r.finished {
			r.finalize(errors.New("transcode stream closed before completion"))
		}
	})
	return closeErr
}

func (r *cachingTranscodeReader) finalize(streamErr error) {
	if r.finished {
		return
	}
	r.finished = true
	_ = r.cacheWriter.CloseWithError(streamErr)
	cacheErr := <-r.cacheResult
	if streamErr == nil {
		streamErr = cacheErr
	}
	r.owner.finishFlight(r.key, streamErr, r.completed)
}

func transcodeCacheKey(source ports.ResolvedSource, options ports.AudioTranscodeOptions) string {
	revision := source.Revision
	if revision == "" {
		revision = strings.Join([]string{
			source.Name, strconv.FormatInt(source.Size, 10), source.ModTime.UTC().Format(time.RFC3339Nano),
		}, ":")
	}
	return domain.StableID("transcode", options.TrackID, revision, options.Format, strconv.Itoa(options.BitRate))
}

func replaceAudioExtension(name, extension string) string {
	current := filepath.Ext(name)
	if current == "" {
		return name + extension
	}
	return strings.TrimSuffix(name, current) + extension
}

func contentTypeForTranscode(format string) string {
	if strings.EqualFold(strings.TrimPrefix(format, "."), "mp3") {
		return "audio/mpeg"
	}
	return "application/octet-stream"
}
