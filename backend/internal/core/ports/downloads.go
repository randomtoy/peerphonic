package ports

import (
	"context"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

// TrackDownloadMonitor reports provider-neutral background cache jobs.
type TrackDownloadMonitor interface {
	TrackDownloads(ctx context.Context) ([]domain.TrackDownload, error)
}

// TrackDownloadController changes provider-owned single-track cache jobs.
// Implementations return ErrNotFound for IDs owned by another provider.
type TrackDownloadController interface {
	TrackDownloadMonitor
	CancelTrackDownload(ctx context.Context, id string) error
	RetryTrackDownload(ctx context.Context, id string) error
}
