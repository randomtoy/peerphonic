package ports

import (
	"context"
	"errors"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

var ErrSourceBusy = errors.New("source is busy")

type SourceManager interface {
	ManagedSources(ctx context.Context) ([]domain.ManagedSource, error)
	PauseSource(ctx context.Context, id string) error
	ResumeSource(ctx context.Context, id string) error
	PinSource(ctx context.Context, id string, pinned bool) error
	RemoveSource(ctx context.Context, id string, deleteData bool) error
}
