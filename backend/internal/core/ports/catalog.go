package ports

import (
	"context"
	"errors"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

var ErrNotFound = errors.New("not found")

// Catalog stores searchable music metadata. Implementations must make
// ReplaceProviderTracks atomic so a failed scan cannot leave a partial catalog.
type Catalog interface {
	ReplaceProviderTracks(ctx context.Context, provider string, tracks []domain.Track) error
	Track(ctx context.Context, id string) (domain.Track, error)
	Artists(ctx context.Context) ([]domain.Artist, error)
	Albums(ctx context.Context, offset, limit int) ([]domain.Album, error)
	AlbumsByArtist(ctx context.Context, artistID string) ([]domain.Album, error)
	TracksByAlbum(ctx context.Context, albumID string) ([]domain.Track, error)
}
