package torrents

import (
	"context"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type trackUpdater interface {
	Track(ctx context.Context, id string) (domain.Track, error)
	UpdateTrack(ctx context.Context, track domain.Track) error
}

type Enricher = scanner.Enricher

func NewEnricher(catalog trackUpdater, extractor scanner.Extractor, artwork scanner.ArtworkWriter) *Enricher {
	return scanner.NewEnricher(catalog, extractor, artwork)
}
