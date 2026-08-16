package ports

import (
	"context"
	"io"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

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

type SourceProvider interface {
	Name() string
	Search(ctx context.Context, query domain.SearchQuery) ([]domain.TrackSource, error)
	Resolve(ctx context.Context, ref domain.SourceRef) (ResolvedSource, error)
}

// BlobStore is binary storage for cached media and artwork. It is intentionally
// independent from Catalog because metadata and media can have different backends.
type BlobStore interface {
	Open(ctx context.Context, key string) (ReadSeekCloser, error)
	Put(ctx context.Context, key string, src io.Reader) error
	Delete(ctx context.Context, key string) error
}
