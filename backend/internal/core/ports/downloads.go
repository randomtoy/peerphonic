package ports

import (
	"context"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

// TrackDownloadMonitor reports provider-neutral background cache jobs.
type TrackDownloadMonitor interface {
	TrackDownloads(ctx context.Context) ([]domain.TrackDownload, error)
}
