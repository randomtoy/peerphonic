package services

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type audioTranscoderStub struct {
	mu     sync.Mutex
	calls  int
	output string
}

func (s *audioTranscoderStub) Transcode(
	_ context.Context, source ports.ResolvedSource, _ ports.AudioTranscodeOptions,
) (ports.TranscodedSource, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	_ = source.Content.Close()
	return ports.TranscodedSource{
		Content: io.NopCloser(bytes.NewBufferString(s.output)),
		Name:    "song.mp3", ContentType: "audio/mpeg",
	}, nil
}

func (s *audioTranscoderStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestCachedAudioTranscoderReusesCompletedVariant(t *testing.T) {
	t.Parallel()

	delegate := &audioTranscoderStub{output: "cached-mp3"}
	transcoder := NewCachedAudioTranscoder(delegate, newTestMediaCache(t, 1<<20))
	options := ports.AudioTranscodeOptions{TrackID: "track-1", Format: "mp3", BitRate: 128}
	first, err := transcoder.Transcode(context.Background(), transcodeTestSource(), options)
	if err != nil {
		t.Fatal(err)
	}
	firstData, err := io.ReadAll(first.Content)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Content.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := transcoder.Transcode(context.Background(), transcodeTestSource(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Content.Close()
	secondData, err := io.ReadAll(second.Content)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstData) != "cached-mp3" || string(secondData) != "cached-mp3" ||
		delegate.callCount() != 1 || !second.Cached || second.Size != int64(len("cached-mp3")) {
		t.Fatalf("first = %q, second = %q, calls = %d, cached = %t, size = %d",
			firstData, secondData, delegate.callCount(), second.Cached, second.Size)
	}
	jobs, err := transcoder.TrackDownloads(context.Background())
	if err != nil || len(jobs) != 1 || jobs[0].State != domain.DownloadStateCached ||
		jobs[0].TrackID != "track-1" || jobs[0].CompletedBytes != int64(len("cached-mp3")) {
		t.Fatalf("TrackDownloads() = %#v, %v", jobs, err)
	}
}

func TestCachedAudioTranscoderDeduplicatesConcurrentVariant(t *testing.T) {
	t.Parallel()

	delegate := &audioTranscoderStub{output: "one-transcode"}
	transcoder := NewCachedAudioTranscoder(delegate, newTestMediaCache(t, 1<<20))
	options := ports.AudioTranscodeOptions{TrackID: "track-1", Format: "mp3"}
	leader, err := transcoder.Transcode(context.Background(), transcodeTestSource(), options)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		source ports.TranscodedSource
		err    error
	}
	waiter := make(chan result, 1)
	go func() {
		source, transcodeErr := transcoder.Transcode(context.Background(), transcodeTestSource(), options)
		waiter <- result{source: source, err: transcodeErr}
	}()
	select {
	case <-waiter:
		t.Fatal("concurrent transcode returned before the leader completed")
	case <-time.After(25 * time.Millisecond):
	}
	if _, err := io.Copy(io.Discard, leader.Content); err != nil {
		t.Fatal(err)
	}
	if err := leader.Content.Close(); err != nil {
		t.Fatal(err)
	}
	completed := <-waiter
	if completed.err != nil {
		t.Fatal(completed.err)
	}
	defer completed.source.Content.Close()
	if !completed.source.Cached || delegate.callCount() != 1 {
		t.Fatalf("waiter cached = %t, calls = %d", completed.source.Cached, delegate.callCount())
	}
}

func TestCachedAudioTranscoderDoesNotKeepPartialVariant(t *testing.T) {
	t.Parallel()

	delegate := &audioTranscoderStub{output: "partial-output"}
	transcoder := NewCachedAudioTranscoder(delegate, newTestMediaCache(t, 1<<20))
	options := ports.AudioTranscodeOptions{TrackID: "track-1", Format: "mp3"}
	partial, err := transcoder.Transcode(context.Background(), transcodeTestSource(), options)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 2)
	if _, err := partial.Content.Read(buffer); err != nil {
		t.Fatal(err)
	}
	if err := partial.Content.Close(); err != nil {
		t.Fatal(err)
	}
	jobs, err := transcoder.TrackDownloads(context.Background())
	if err != nil || len(jobs) != 1 || jobs[0].State != domain.DownloadStateFailed {
		t.Fatalf("partial TrackDownloads() = %#v, %v", jobs, err)
	}
	complete, err := transcoder.Transcode(context.Background(), transcodeTestSource(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer complete.Content.Close()
	if _, err := io.Copy(io.Discard, complete.Content); err != nil {
		t.Fatal(err)
	}
	if delegate.callCount() != 2 {
		t.Fatalf("delegate calls = %d, want 2", delegate.callCount())
	}
}

func transcodeTestSource() ports.ResolvedSource {
	return ports.ResolvedSource{
		Content: &testReadSeekCloser{Reader: bytes.NewReader([]byte("source"))},
		Name:    "song.flac", ContentType: "audio/flac", Size: 6, Revision: "source-v1",
	}
}

type testReadSeekCloser struct {
	*bytes.Reader
}

func (*testReadSeekCloser) Close() error { return nil }
