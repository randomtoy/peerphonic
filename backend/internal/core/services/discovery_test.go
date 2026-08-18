package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type discoveryProviderStub struct {
	results []domain.TrackSource
	err     error
}

func (s discoveryProviderStub) Name() string { return "remote" }

func (s discoveryProviderStub) Search(
	_ context.Context, _ domain.SearchQuery,
) ([]domain.TrackSource, error) {
	return s.results, s.err
}

type trackSourceWriterStub struct {
	saved []domain.TrackSource
	err   error
}

func (s *trackSourceWriterStub) SaveTrackSource(_ context.Context, source domain.TrackSource) error {
	s.saved = append(s.saved, source)
	return s.err
}

func TestDiscoveryServiceAddsSelectedSearchResult(t *testing.T) {
	t.Parallel()

	result := domain.TrackSource{
		Track:        domain.Track{ID: "track-1", Title: "One"},
		Ref:          domain.SourceRef{Provider: "remote", Key: "source-1"},
		DiscoveredAt: time.Now().UTC(),
	}
	writer := &trackSourceWriterStub{}
	service := NewDiscoveryService(discoveryProviderStub{results: []domain.TrackSource{result}}, writer)

	results, err := service.Search(context.Background(), domain.SearchQuery{Text: "One"})
	if err != nil || len(results) != 1 {
		t.Fatalf("Search() = %#v, %v", results, err)
	}
	track, err := service.Add(context.Background(), result.Track.ID)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if track != result.Track || len(writer.saved) != 1 || writer.saved[0].Ref != result.Ref {
		t.Fatalf("Add() = %#v, saved = %#v", track, writer.saved)
	}
}

func TestDiscoveryServiceRejectsUnknownAndPropagatesStorageErrors(t *testing.T) {
	t.Parallel()

	result := domain.TrackSource{
		Track:        domain.Track{ID: "track-1"},
		Ref:          domain.SourceRef{Provider: "remote", Key: "source-1"},
		DiscoveredAt: time.Now().UTC(),
	}
	writer := &trackSourceWriterStub{err: errors.New("storage unavailable")}
	service := NewDiscoveryService(discoveryProviderStub{results: []domain.TrackSource{result}}, writer)
	if _, err := service.Add(context.Background(), "missing"); !errors.Is(err, ErrDiscoveryResultNotFound) {
		t.Fatalf("Add(missing) error = %v", err)
	}
	if _, err := service.Search(context.Background(), domain.SearchQuery{Text: "One"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(context.Background(), result.Track.ID); err == nil || errors.Is(err, ErrDiscoveryResultNotFound) {
		t.Fatalf("Add() error = %v", err)
	}
}
