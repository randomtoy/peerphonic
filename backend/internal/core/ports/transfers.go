package ports

import (
	"context"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

// SourceTransferMonitor reports live provider activity without exposing a
// concrete transport or P2P implementation to application-facing adapters.
type SourceTransferMonitor interface {
	Transfers(ctx context.Context) ([]domain.SourceTransfer, error)
}
