package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type discoveryProviderStub struct {
	results     []domain.TrackSource
	collection  domain.SourceCollection
	err         error
	browseCalls *int
	search      func(domain.SearchQuery) ([]domain.TrackSource, error)
}

func (s discoveryProviderStub) Name() string { return "remote" }

func (s discoveryProviderStub) Search(_ context.Context, query domain.SearchQuery) ([]domain.TrackSource, error) {
	if s.search != nil {
		return s.search(query)
	}
	return s.results, s.err
}

func (s discoveryProviderStub) BrowseCollection(
	_ context.Context, _ domain.TrackSource,
) (domain.SourceCollection, error) {
	if s.browseCalls != nil {
		(*s.browseCalls)++
	}
	return s.collection, s.err
}

type trackSourceWriterStub struct {
	saved []domain.TrackSource
	err   error
}

func (s *trackSourceWriterStub) SaveTrackSource(_ context.Context, source domain.TrackSource) error {
	s.saved = append(s.saved, source)
	return s.err
}

func (s *trackSourceWriterStub) SaveTrackSources(_ context.Context, sources []domain.TrackSource) error {
	s.saved = append(s.saved, sources...)
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
	cached, ok := service.Result(result.Track.ID)
	if !ok || cached != result {
		t.Fatalf("Result() = %#v, %v", cached, ok)
	}
	if _, ok := service.Result("missing"); ok {
		t.Fatal("Result(missing) unexpectedly succeeded")
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

func TestDiscoveryServiceAddsRankedAlternativeSourcesForPlayback(t *testing.T) {
	t.Parallel()

	selected := domain.TrackSource{
		Track: domain.Track{
			ID: "track-1", Artist: "System of a Down", Title: "01 - Prison Song",
			Suffix: "mp3", Duration: 3 * time.Minute,
		},
		Ref:          domain.SourceRef{Provider: "remote", Key: "stale-peer"},
		DiscoveredAt: time.Now().Add(-time.Minute).UTC(),
	}
	fast := domain.TrackSource{
		Track: domain.Track{Artist: "System Of A Down", Title: "Prison Song", Suffix: "mp3", Duration: 181 * time.Second},
		Ref:   domain.SourceRef{Provider: "remote", Key: "fast-peer"}, DisplayPath: "System Of A Down/Toxicity/01 Prison Song.mp3",
		Availability: domain.SourceAvailability{FreeUploadSlot: true, UploadSpeed: 2_000_000},
	}
	queued := domain.TrackSource{
		Track: domain.Track{Artist: "System Of A Down", Title: "Prison Song", Suffix: "mp3", Duration: 179 * time.Second},
		Ref:   domain.SourceRef{Provider: "remote", Key: "queued-peer"}, DisplayPath: "System Of A Down/Toxicity/Prison Song.mp3",
		Availability: domain.SourceAvailability{QueueLength: 12, UploadSpeed: 5_000_000},
	}
	wrong := domain.TrackSource{
		Track: domain.Track{Artist: "The Outsiders", Title: "Prison Song", Suffix: "mp3"},
		Ref:   domain.SourceRef{Provider: "remote", Key: "wrong-song"}, DisplayPath: "The Outsiders/Prison Song.mp3",
	}
	locked := fast
	locked.Ref.Key = "locked-peer"
	locked.Availability.RequiresApproval = true
	live := fast
	live.Track.Title = "Prison Song (Live)"
	live.Ref.Key = "live-peer"
	live.DisplayPath = "System Of A Down/Live/Prison Song (Live).mp3"

	var queries []domain.SearchQuery
	provider := discoveryProviderStub{search: func(query domain.SearchQuery) ([]domain.TrackSource, error) {
		queries = append(queries, query)
		if len(queries) == 1 {
			return []domain.TrackSource{selected}, nil
		}
		return []domain.TrackSource{queued, wrong, locked, live, fast}, nil
	}}
	writer := &trackSourceWriterStub{}
	service := NewDiscoveryService(provider, writer)
	if _, err := service.Search(context.Background(), domain.SearchQuery{Text: "system of a down"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(context.Background(), selected.Track.ID); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 || queries[1].Text != "system prison song" || queries[1].Limit != 100 {
		t.Fatalf("alternative query = %#v", queries)
	}
	if len(writer.saved) != 3 {
		t.Fatalf("saved alternatives = %#v", writer.saved)
	}
	if writer.saved[0].Ref != fast.Ref || writer.saved[1].Ref != queued.Ref || writer.saved[2].Ref != selected.Ref {
		t.Fatalf("ranked sources = %#v", writer.saved)
	}
	for _, source := range writer.saved {
		if source.Track != selected.Track {
			t.Fatalf("alternative track identity = %#v, want %#v", source.Track, selected.Track)
		}
	}
}

func TestDiscoveryServiceAddsACollectionAsOneBatch(t *testing.T) {
	t.Parallel()

	anchor := domain.TrackSource{
		Track: domain.Track{ID: "track-1"}, Ref: domain.SourceRef{Provider: "remote", Key: "one"},
	}
	collection := domain.SourceCollection{Name: "Album", Tracks: []domain.TrackSource{
		anchor,
		{Track: domain.Track{ID: "track-2"}, Ref: domain.SourceRef{Provider: "remote", Key: "two"}},
	}}
	writer := &trackSourceWriterStub{}
	browseCalls := 0
	service := NewDiscoveryService(discoveryProviderStub{
		results: []domain.TrackSource{anchor}, collection: collection, browseCalls: &browseCalls,
	}, writer)
	if _, err := service.Search(context.Background(), domain.SearchQuery{Text: "Album"}); err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewCollection(context.Background(), anchor.Track.ID)
	if err != nil || preview.Name != "Album" {
		t.Fatalf("PreviewCollection() = %#v, %v", preview, err)
	}
	got, err := service.AddCollection(context.Background(), anchor.Track.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Album" || len(writer.saved) != 2 || writer.saved[1].Track.ID != "track-2" || browseCalls != 1 {
		t.Fatalf("AddCollection() = %#v, saved = %#v, browse calls = %d", got, writer.saved, browseCalls)
	}
}
