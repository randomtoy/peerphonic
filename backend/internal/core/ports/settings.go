package ports

import (
	"context"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type TransferLimitStore interface {
	TransferLimits(ctx context.Context) (domain.TransferLimits, error)
	SaveTransferLimits(ctx context.Context, limits domain.TransferLimits) error
}

type TransferLimitController interface {
	TransferLimits(ctx context.Context) (domain.TransferLimits, error)
	SetTransferLimits(ctx context.Context, limits domain.TransferLimits) error
}

type TransferSettingsManager interface {
	Limits(ctx context.Context, actor domain.User) (domain.TransferLimits, error)
	UpdateLimits(ctx context.Context, actor domain.User, limits domain.TransferLimits) (domain.TransferLimits, error)
}
