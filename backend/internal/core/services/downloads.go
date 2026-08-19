package services

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

// DownloadService combines provider-owned cache jobs behind one API-facing
// monitor and routes mutations only to the provider that owns the job ID.
type DownloadService struct {
	monitors []ports.TrackDownloadMonitor
}

func NewDownloadService(monitors ...ports.TrackDownloadMonitor) *DownloadService {
	return &DownloadService{monitors: monitors}
}

func (s *DownloadService) TrackDownloads(ctx context.Context) ([]domain.TrackDownload, error) {
	var result []domain.TrackDownload
	for _, monitor := range s.monitors {
		items, err := monitor.TrackDownloads(ctx)
		if err != nil {
			return nil, fmt.Errorf("read provider track downloads: %w", err)
		}
		result = append(result, items...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].Name < result[j].Name
		}
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result, nil
}

func (s *DownloadService) CancelTrackDownload(ctx context.Context, id string) error {
	return s.change(ctx, id, func(controller ports.TrackDownloadController) error {
		return controller.CancelTrackDownload(ctx, id)
	})
}

func (s *DownloadService) RetryTrackDownload(ctx context.Context, id string) error {
	return s.change(ctx, id, func(controller ports.TrackDownloadController) error {
		return controller.RetryTrackDownload(ctx, id)
	})
}

func (s *DownloadService) change(
	ctx context.Context, id string, operation func(ports.TrackDownloadController) error,
) error {
	for _, monitor := range s.monitors {
		controller, ok := monitor.(ports.TrackDownloadController)
		if !ok {
			continue
		}
		err := operation(controller)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ports.ErrNotFound) {
			return err
		}
	}
	return ports.ErrNotFound
}
