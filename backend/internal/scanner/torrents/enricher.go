package torrents

import (
	"context"
	"fmt"
	"os"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type metadataExtractor interface {
	Extract(path string, info os.FileInfo) (scanner.Metadata, error)
}

type trackUpdater interface {
	Track(ctx context.Context, id string) (domain.Track, error)
	UpdateTrack(ctx context.Context, track domain.Track) error
}

type Enricher struct {
	catalog   trackUpdater
	extractor metadataExtractor
	artwork   scanner.ArtworkWriter
}

func NewEnricher(catalog trackUpdater, extractor metadataExtractor, artwork scanner.ArtworkWriter) *Enricher {
	return &Enricher{catalog: catalog, extractor: extractor, artwork: artwork}
}

func (e *Enricher) Enrich(ctx context.Context, trackID, mediaPath string) error {
	track, err := e.catalog.Track(ctx, trackID)
	if err != nil {
		return fmt.Errorf("find completed torrent track: %w", err)
	}
	if err := enrichTrack(ctx, &track, mediaPath, e.extractor, e.artwork); err != nil {
		return err
	}
	if err := e.catalog.UpdateTrack(ctx, track); err != nil {
		return fmt.Errorf("update completed torrent metadata: %w", err)
	}
	return nil
}

func enrichTrack(
	ctx context.Context,
	track *domain.Track,
	mediaPath string,
	extractor metadataExtractor,
	artwork scanner.ArtworkWriter,
) error {
	info, err := os.Stat(mediaPath)
	if err != nil {
		return fmt.Errorf("stat completed torrent track: %w", err)
	}
	if !info.Mode().IsRegular() || (track.Size > 0 && info.Size() != track.Size) {
		return fmt.Errorf("completed torrent track has unexpected size %d, want %d", info.Size(), track.Size)
	}
	metadata, err := extractor.Extract(mediaPath, info)
	if err != nil {
		return fmt.Errorf("extract completed torrent metadata: %w", err)
	}
	albumName := track.Album
	albumID := track.AlbumID
	albumArtist := track.AlbumArtist
	albumArtistID := track.AlbumArtistID
	scanner.ApplyMetadata(track, metadata)
	// A single completed track must not split away from its provisional album.
	// Album identity can be upgraded later as one atomic group operation.
	track.Album = albumName
	track.AlbumID = albumID
	track.AlbumArtist = albumArtist
	track.AlbumArtistID = albumArtistID
	if metadata.Artwork != nil && artwork != nil {
		coverArtID, err := artwork.Put(ctx, metadata.Artwork.Data)
		if err != nil {
			return fmt.Errorf("store completed torrent artwork: %w", err)
		}
		track.CoverArtID = coverArtID
	}
	return nil
}
