package torrents

import (
	"context"
	"fmt"

	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type SourceManager struct {
	provider *torrentprovider.Provider
	scans    scanController
}

func NewSourceManager(provider *torrentprovider.Provider, scans scanController) *SourceManager {
	return &SourceManager{provider: provider, scans: scans}
}

func (m *SourceManager) ManagedSources(ctx context.Context) ([]domain.ManagedSource, error) {
	return m.provider.ManagedSources(ctx)
}

func (m *SourceManager) PauseSource(ctx context.Context, id string) error {
	return m.provider.PauseSource(ctx, id)
}

func (m *SourceManager) ResumeSource(ctx context.Context, id string) error {
	return m.provider.ResumeSource(ctx, id)
}

func (m *SourceManager) PinSource(ctx context.Context, id string, pinned bool) error {
	return m.provider.PinSource(ctx, id, pinned)
}

func (m *SourceManager) RemoveSource(ctx context.Context, id string, deleteData bool) error {
	if err := m.provider.RemoveSource(ctx, id, deleteData); err != nil {
		return err
	}
	if _, err := m.scans.ScanNow(ctx); err != nil {
		return fmt.Errorf("synchronize catalog after source removal: %w", err)
	}
	return nil
}
