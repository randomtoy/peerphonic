package services

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type catalogStub struct {
	track domain.Track
	err   error
}

func (c catalogStub) ReplaceProviderTracks(context.Context, string, []domain.Track) error { return nil }
func (c catalogStub) Track(context.Context, string) (domain.Track, error)                 { return c.track, c.err }
func (c catalogStub) Artists(context.Context) ([]domain.Artist, error)                    { return nil, nil }
func (c catalogStub) AlbumsByArtist(context.Context, string) ([]domain.Album, error)      { return nil, nil }
func (c catalogStub) TracksByAlbum(context.Context, string) ([]domain.Track, error)       { return nil, nil }

type providerStub struct {
	resolved domain.SourceRef
}

func (p *providerStub) Name() string { return "local" }
func (p *providerStub) Search(context.Context, domain.SearchQuery) ([]domain.TrackSource, error) {
	return nil, nil
}
func (p *providerStub) Resolve(_ context.Context, ref domain.SourceRef) (ports.ResolvedSource, error) {
	p.resolved = ref
	return ports.ResolvedSource{Content: seekCloser{Reader: strings.NewReader("audio")}}, nil
}

type seekCloser struct{ *strings.Reader }

func (seekCloser) Close() error { return nil }

func TestStreamingServiceResolvesThroughTrackProvider(t *testing.T) {
	t.Parallel()

	provider := &providerStub{}
	ref := domain.SourceRef{Provider: "local", Key: "album/song.mp3"}
	service := NewStreamingService(catalogStub{track: domain.Track{ID: "track-1", Source: ref}}, provider)

	stream, err := service.Open(context.Background(), "track-1")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Content.Close()
	if provider.resolved != ref {
		t.Fatalf("resolved ref = %#v, want %#v", provider.resolved, ref)
	}
	data, _ := io.ReadAll(stream.Content)
	if string(data) != "audio" {
		t.Fatalf("stream = %q, want audio", data)
	}
}

func TestStreamingServiceRejectsUnknownProvider(t *testing.T) {
	t.Parallel()

	service := NewStreamingService(catalogStub{track: domain.Track{Source: domain.SourceRef{Provider: "remote"}}})
	_, err := service.Open(context.Background(), "track-1")
	if err == nil || errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Open() error = %v, want provider error", err)
	}
}
