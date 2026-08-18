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
	UpdateTrack(ctx context.Context, track domain.Track) error
	Sources(ctx context.Context, trackID string) ([]domain.SourceRef, error)
	Artist(ctx context.Context, id string) (domain.Artist, error)
	Artists(ctx context.Context) ([]domain.Artist, error)
	Genres(ctx context.Context) ([]domain.Genre, error)
	Albums(ctx context.Context, query AlbumListQuery) ([]domain.Album, error)
	AlbumsByArtist(ctx context.Context, artistID string) ([]domain.Album, error)
	TracksByAlbum(ctx context.Context, albumID string) ([]domain.Track, error)
	TracksByGenre(ctx context.Context, genre string, offset, limit int) ([]domain.Track, error)
	RandomTracks(ctx context.Context, query RandomTracksQuery) ([]domain.Track, error)
	Search(ctx context.Context, query CatalogSearch) (CatalogSearchResult, error)
	Playlists(ctx context.Context, owner string) ([]domain.Playlist, error)
	Playlist(ctx context.Context, id string) (domain.Playlist, error)
	SavePlaylist(ctx context.Context, playlist domain.Playlist) error
	DeletePlaylist(ctx context.Context, id string) error
}

// TrackSourceWriter incrementally persists a selected provider result without
// replacing the provider's complete catalog.
type TrackSourceWriter interface {
	SaveTrackSource(ctx context.Context, source domain.TrackSource) error
	SaveTrackSources(ctx context.Context, sources []domain.TrackSource) error
}

type AlbumOrder string

const (
	AlbumOrderName     AlbumOrder = "name"
	AlbumOrderArtist   AlbumOrder = "artist"
	AlbumOrderNewest   AlbumOrder = "newest"
	AlbumOrderRandom   AlbumOrder = "random"
	AlbumOrderYearAsc  AlbumOrder = "year-asc"
	AlbumOrderYearDesc AlbumOrder = "year-desc"
)

type AlbumListQuery struct {
	Offset   int
	Limit    int
	Order    AlbumOrder
	FromYear int
	ToYear   int
	Genre    string
}

type AlbumAlias struct {
	AliasID string
	TrackID string
}

type RandomTracksQuery struct {
	Limit    int
	Genre    string
	FromYear int
	ToYear   int
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
