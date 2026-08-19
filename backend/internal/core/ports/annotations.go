package ports

import (
	"context"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

// MediaAnnotationStore persists user-specific library state independently of
// the physical media sources backing catalog entries.
type MediaAnnotationStore interface {
	MediaAnnotations(ctx context.Context, owner string) ([]domain.MediaAnnotation, error)
	UpdateMediaAnnotations(ctx context.Context, updates []MediaAnnotationUpdate) error
}

type MediaAnnotationUpdate struct {
	Owner          string
	Media          domain.MediaRef
	StarredAt      *time.Time
	Rating         *int
	PlayCountDelta int64
	LastPlayed     *time.Time
}
