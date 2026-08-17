package torrents

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type enrichmentExtractor struct {
	metadata scanner.Metadata
}

func (e enrichmentExtractor) Extract(string, os.FileInfo) (scanner.Metadata, error) {
	return e.metadata, nil
}

type enrichmentArtwork struct {
	id string
}

func (w enrichmentArtwork) Put(context.Context, []byte) (string, error) {
	return w.id, nil
}

func TestEnricherUpdatesCompletedTrackWithoutChangingItsSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	media := []byte("complete torrent track")
	mediaPath := filepath.Join(t.TempDir(), "01 Song.mp3")
	if err := os.WriteFile(mediaPath, media, 0o600); err != nil {
		t.Fatal(err)
	}
	track := domain.Track{
		ID: "track-remote", Title: "01 Song", Artist: "Folder Artist", ArtistID: "artist-old",
		Album: "Folder Album", AlbumID: "album-old", AlbumArtist: "Folder Artist",
		Size: int64(len(media)), Suffix: "mp3", ContentType: "audio/mpeg",
	}
	ref := domain.SourceRef{Provider: "torrent", Key: "hash/01 Song.mp3"}
	if err := catalog.ReplaceProviderTracks(ctx, "torrent", []domain.TrackSource{{
		Track: track, Ref: ref,
	}}, nil); err != nil {
		t.Fatal(err)
	}
	enricher := NewEnricher(catalog, enrichmentExtractor{metadata: scanner.Metadata{
		Title: "Tagged Song", Artist: "Tagged Artist", Album: "Tagged Album",
		AlbumArtist: "Tagged Album Artist", TrackNumber: 1, DiscNumber: 2, Year: 2020,
		Duration: 4 * time.Minute, Size: int64(len(media)), BitRate: 320,
		Suffix: "mp3", ContentType: "audio/mpeg", Artwork: &scanner.Artwork{Data: []byte("image")},
	}}, enrichmentArtwork{id: "art-enriched"})
	if err := enricher.Enrich(ctx, track.ID, mediaPath); err != nil {
		t.Fatal(err)
	}

	updated, err := catalog.Track(ctx, track.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Tagged Song" || updated.Artist != "Tagged Artist" ||
		updated.Album != "Folder Album" || updated.AlbumID != "album-old" ||
		updated.TrackNumber != 1 || updated.DiscNumber != 2 ||
		updated.Year != 2020 || updated.Duration != 4*time.Minute || updated.BitRate != 320 ||
		updated.CoverArtID != "art-enriched" {
		t.Fatalf("updated track = %#v", updated)
	}
	sources, err := catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 1 || sources[0] != ref {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
	aliasTracks, err := catalog.TracksByAlbum(ctx, "album-old")
	if err != nil || len(aliasTracks) != 1 || aliasTracks[0].ID != track.ID {
		t.Fatalf("TracksByAlbum(old) = %#v, %v", aliasTracks, err)
	}
}
