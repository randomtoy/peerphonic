package scanner

import (
	"context"
	"fmt"
	"os"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type enrichmentCatalog interface {
	Track(ctx context.Context, id string) (domain.Track, error)
	UpdateTrack(ctx context.Context, track domain.Track) error
}

// Enricher replaces provisional remote-file metadata after one track has been
// materialized, while preserving its existing album identity and source refs.
type Enricher struct {
	catalog   enrichmentCatalog
	extractor Extractor
	artwork   ArtworkWriter
}

func NewEnricher(catalog enrichmentCatalog, extractor Extractor, artwork ArtworkWriter) *Enricher {
	return &Enricher{catalog: catalog, extractor: extractor, artwork: artwork}
}

func (e *Enricher) Enrich(ctx context.Context, trackID, mediaPath string) error {
	return e.EnrichWithExpectedSize(ctx, trackID, mediaPath, 0)
}

// EnrichWithExpectedSize accepts the size of the physical source that actually
// completed. Equivalent remote copies may use a different encoding and size
// than the provisional source first shown in the catalog.
func (e *Enricher) EnrichWithExpectedSize(
	ctx context.Context, trackID, mediaPath string, expectedSize int64,
) error {
	track, err := e.catalog.Track(ctx, trackID)
	if err != nil {
		return fmt.Errorf("find completed track: %w", err)
	}
	if expectedSize > 0 {
		track.Size = expectedSize
	}
	if err := EnrichTrack(ctx, &track, mediaPath, e.extractor, e.artwork); err != nil {
		return err
	}
	if err := e.catalog.UpdateTrack(ctx, track); err != nil {
		return fmt.Errorf("update completed track metadata: %w", err)
	}
	return nil
}

// EnrichTrack applies metadata from one complete media file to an existing
// catalog track without changing its provisional album grouping.
func EnrichTrack(
	ctx context.Context,
	track *domain.Track,
	mediaPath string,
	extractor Extractor,
	artwork ArtworkWriter,
) error {
	info, err := os.Stat(mediaPath)
	if err != nil {
		return fmt.Errorf("stat completed track: %w", err)
	}
	if !info.Mode().IsRegular() || (track.Size > 0 && info.Size() != track.Size) {
		return fmt.Errorf("completed track has unexpected size %d, want %d", info.Size(), track.Size)
	}
	metadata, err := extractor.Extract(mediaPath, info)
	if err != nil {
		return fmt.Errorf("extract completed track metadata: %w", err)
	}
	albumName := track.Album
	albumID := track.AlbumID
	albumArtist := track.AlbumArtist
	albumArtistID := track.AlbumArtistID
	ApplyMetadata(track, metadata)
	// One completed track must not split away from its provisional album. Album
	// identity can be upgraded later as one atomic group operation.
	track.Album = albumName
	track.AlbumID = albumID
	track.AlbumArtist = albumArtist
	track.AlbumArtistID = albumArtistID
	if metadata.Artwork != nil && artwork != nil {
		coverArtID, err := artwork.Put(ctx, metadata.Artwork.Data)
		if err != nil {
			return fmt.Errorf("store completed track artwork: %w", err)
		}
		track.CoverArtID = coverArtID
	}
	return nil
}
