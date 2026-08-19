package ports

import (
	"context"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type AuditStore interface {
	AppendAuditEntry(ctx context.Context, entry domain.AuditEntry) error
	AuditEntries(ctx context.Context, limit int) ([]domain.AuditEntry, error)
}
