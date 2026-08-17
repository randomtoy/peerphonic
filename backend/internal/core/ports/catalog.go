package ports

import (
	"context"
	"errors"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

var ErrNotFound = errors.New("not found")

// Catalog stores searchable music metadata and its source references.
// Implementations must make ReplaceProviderTracks atomic so a failed scan
// cannot leave a partial catalog.
type Catalog interface {
	ReplaceProviderTracks(ctx context.Context, provider string, tracks []domain.TrackSource, albumAliases []AlbumAlias) error
	Track(ctx context.Context, id string) (domain.Track, error)
	Sources(ctx context.Context, trackID string) ([]domain.SourceRef, error)
	Artist(ctx context.Context, id string) (domain.Artist, error)
	Artists(ctx context.Context) ([]domain.Artist, error)
	Albums(ctx context.Context, offset, limit int) ([]domain.Album, error)
	AlbumsByArtist(ctx context.Context, artistID string) ([]domain.Album, error)
	TracksByAlbum(ctx context.Context, albumID string) ([]domain.Track, error)
	Search(ctx context.Context, query CatalogSearch) (CatalogSearchResult, error)
}

type AlbumAlias struct {
	AliasID string
	TrackID string
}

type CatalogSearch struct {
	Text         string
	ArtistOffset int
	ArtistCount  int
	AlbumOffset  int
	AlbumCount   int
	SongOffset   int
	SongCount    int
}

type CatalogSearchResult struct {
	Artists []domain.Artist
	Albums  []domain.Album
	Songs   []domain.Track
}
