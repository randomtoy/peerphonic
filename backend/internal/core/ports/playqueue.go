package ports

import (
	"context"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type PlayQueueStore interface {
	PlayQueue(ctx context.Context, owner string) (domain.PlayQueue, error)
	SavePlayQueue(ctx context.Context, queue domain.PlayQueue) error
}
