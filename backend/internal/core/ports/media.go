package ports

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

var ErrSourceUnavailable = errors.New("source unavailable")

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type ResolvedSource struct {
	Content     ReadSeekCloser
	Name        string
	ContentType string
	Size        int64
	ModTime     time.Time
}

type AudioTranscodeOptions struct {
	Format  string
	BitRate int
}

type TranscodedSource struct {
	Content     io.ReadCloser
	Name        string
	ContentType string
}

// AudioTranscoder converts a resolved source while it is being read. The
// implementation owns and closes source.Content after Transcode succeeds.
type AudioTranscoder interface {
	Transcode(ctx context.Context, source ResolvedSource, options AudioTranscodeOptions) (TranscodedSource, error)
}

type SourceSearcher interface {
	Name() string
	Search(ctx context.Context, query domain.SearchQuery) ([]domain.TrackSource, error)
}

// SourceCollectionBrowser discovers tracks grouped with an already discovered
// source. The source remains opaque to the application layer.
type SourceCollectionBrowser interface {
	BrowseCollection(ctx context.Context, anchor domain.TrackSource) (domain.SourceCollection, error)
}

type SourceProvider interface {
	SourceSearcher
	Resolve(ctx context.Context, trackID string, ref domain.SourceRef) (ResolvedSource, error)
}

// BlobStore is binary storage for cached media and artwork. It is intentionally
// independent from Catalog because metadata and media can have different backends.
type BlobStore interface {
	Open(ctx context.Context, key string) (ReadSeekCloser, error)
	Put(ctx context.Context, key string, src io.Reader) error
	Delete(ctx context.Context, key string) error
}

// CacheMetadataStore persists cache state independently from media bytes.
type CacheMetadataStore interface {
	CacheEntry(ctx context.Context, key string) (domain.CacheEntry, error)
	CacheEntries(ctx context.Context) ([]domain.CacheEntry, error)
	SaveCacheEntry(ctx context.Context, entry domain.CacheEntry) error
	DeleteCacheEntry(ctx context.Context, key string) error
}
