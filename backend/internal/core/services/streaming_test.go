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
	track      domain.Track
	sources    []domain.SourceRef
	sourcesErr error
}

func (c catalogStub) ReplaceProviderTracks(context.Context, string, []domain.TrackSource, []ports.AlbumAlias) error {
	return nil
}
func (c catalogStub) Track(context.Context, string) (domain.Track, error) { return c.track, nil }
func (c catalogStub) UpdateTrack(context.Context, domain.Track) error     { return nil }
func (c catalogStub) Sources(context.Context, string) ([]domain.SourceRef, error) {
	return c.sources, c.sourcesErr
}
func (c catalogStub) Artist(context.Context, string) (domain.Artist, error) {
	return domain.Artist{}, nil
}
func (c catalogStub) Artists(context.Context) ([]domain.Artist, error)               { return nil, nil }
func (c catalogStub) Albums(context.Context, int, int) ([]domain.Album, error)       { return nil, nil }
func (c catalogStub) AlbumsByArtist(context.Context, string) ([]domain.Album, error) { return nil, nil }
func (c catalogStub) TracksByAlbum(context.Context, string) ([]domain.Track, error)  { return nil, nil }
func (c catalogStub) Search(context.Context, ports.CatalogSearch) (ports.CatalogSearchResult, error) {
	return ports.CatalogSearchResult{}, nil
}

type providerStub struct {
	providerName string
	resolved     domain.SourceRef
	err          error
	calls        int
}

func (p *providerStub) Name() string { return p.providerName }
func (p *providerStub) Search(context.Context, domain.SearchQuery) ([]domain.TrackSource, error) {
	return nil, nil
}
func (p *providerStub) Resolve(_ context.Context, ref domain.SourceRef) (ports.ResolvedSource, error) {
	p.calls++
	p.resolved = ref
	if p.err != nil {
		return ports.ResolvedSource{}, p.err
	}
	return ports.ResolvedSource{Content: seekCloser{Reader: strings.NewReader("audio")}}, nil
}

type seekCloser struct{ *strings.Reader }

func (seekCloser) Close() error { return nil }

func TestStreamingServiceResolvesThroughTrackProvider(t *testing.T) {
	t.Parallel()

	provider := &providerStub{providerName: "local"}
	ref := domain.SourceRef{Provider: "local", Key: "album/song.mp3"}
	service := NewStreamingService(catalogStub{sources: []domain.SourceRef{ref}}, provider)

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

	service := NewStreamingService(catalogStub{sources: []domain.SourceRef{{Provider: "remote", Key: "song"}}})
	_, err := service.Open(context.Background(), "track-1")
	if err == nil || errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Open() error = %v, want provider error", err)
	}
}

func TestStreamingServiceFallsBackInProviderOrder(t *testing.T) {
	t.Parallel()

	local := &providerStub{providerName: "local", err: errors.New("file disappeared")}
	remote := &providerStub{providerName: "remote"}
	service := NewStreamingService(catalogStub{sources: []domain.SourceRef{
		{Provider: "remote", Key: "remote-song"},
		{Provider: "local", Key: "local-song"},
	}}, local, remote)

	stream, err := service.Open(context.Background(), "track-1")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Content.Close()
	if local.calls != 1 || remote.calls != 1 {
		t.Fatalf("resolve calls = local %d, remote %d; want 1 each", local.calls, remote.calls)
	}
	if remote.resolved.Key != "remote-song" {
		t.Fatalf("remote resolved = %#v", remote.resolved)
	}
}
