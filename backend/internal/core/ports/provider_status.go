package ports

import (
	"context"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type ProviderStatusMonitor interface {
	ProviderStatus(ctx context.Context) domain.ProviderStatus
}
