package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestMediaAnnotationsPersistWithoutCatalogEntries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.db")
	catalog, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 12, 30, 0, 123, time.UTC)
	annotation := domain.MediaAnnotation{
		Owner: "alice", Media: domain.MediaRef{Type: domain.MediaSong, ID: "track-1"},
		StarredAt: now, Rating: 4, PlayCount: 2, LastPlayed: now,
	}
	rating := 4
	if err := catalog.UpdateMediaAnnotations(ctx, []ports.MediaAnnotationUpdate{{
		Owner: annotation.Owner, Media: annotation.Media, StarredAt: &now, Rating: &rating,
		PlayCountDelta: 2, LastPlayed: &now,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Close(); err != nil {
		t.Fatal(err)
	}
	catalog, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	got, err := catalog.MediaAnnotations(ctx, "alice")
	if err != nil || len(got) != 1 || got[0] != annotation {
		t.Fatalf("MediaAnnotations() = %#v, %v", got, err)
	}
	zeroTime := time.Time{}
	zeroRating := 0
	if err := catalog.UpdateMediaAnnotations(ctx, []ports.MediaAnnotationUpdate{{
		Owner: annotation.Owner, Media: annotation.Media, StarredAt: &zeroTime, Rating: &zeroRating,
		PlayCountDelta: -2, LastPlayed: &zeroTime,
	}}); err != nil {
		t.Fatal(err)
	}
	got, err = catalog.MediaAnnotations(ctx, "alice")
	if err != nil || len(got) != 0 {
		t.Fatalf("MediaAnnotations() after clearing = %#v, %v", got, err)
	}
}
