package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func (c *Catalog) TransferLimits(ctx context.Context) (domain.TransferLimits, error) {
	var limits domain.TransferLimits
	err := c.db.QueryRowContext(ctx, `SELECT upload_limit_bytes_per_second,
		download_limit_bytes_per_second FROM torrent_transfer_settings WHERE id = 1`).Scan(
		&limits.UploadBytesPerSecond, &limits.DownloadBytesPerSecond,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TransferLimits{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.TransferLimits{}, fmt.Errorf("read torrent transfer settings: %w", err)
	}
	return limits, nil
}

func (c *Catalog) SaveTransferLimits(ctx context.Context, limits domain.TransferLimits) error {
	_, err := c.db.ExecContext(ctx, `INSERT INTO torrent_transfer_settings (
		id, upload_limit_bytes_per_second, download_limit_bytes_per_second, updated_at
	) VALUES (1, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET
		upload_limit_bytes_per_second = excluded.upload_limit_bytes_per_second,
		download_limit_bytes_per_second = excluded.download_limit_bytes_per_second,
		updated_at = excluded.updated_at`,
		limits.UploadBytesPerSecond, limits.DownloadBytesPerSecond,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save torrent transfer settings: %w", err)
	}
	return nil
}
