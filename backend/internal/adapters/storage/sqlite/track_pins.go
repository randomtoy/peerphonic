package sqlite

import (
	"context"
	"fmt"
	"time"
)

func (c *Catalog) SetTrackPinned(ctx context.Context, trackID string, pinned bool) error {
	resolved, err := c.resolveTrackID(ctx, trackID)
	if err != nil {
		return err
	}
	if pinned {
		_, err = c.db.ExecContext(ctx, `INSERT INTO pinned_tracks(track_id, pinned_at)
			VALUES (?, ?) ON CONFLICT(track_id) DO UPDATE SET pinned_at=excluded.pinned_at`,
			resolved, time.Now().UTC().Format(time.RFC3339Nano))
	} else {
		_, err = c.db.ExecContext(ctx, "DELETE FROM pinned_tracks WHERE track_id = ?", resolved)
	}
	if err != nil {
		return fmt.Errorf("set track pin: %w", err)
	}
	return nil
}

func (c *Catalog) PinnedTracks(ctx context.Context) ([]string, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT track_id FROM pinned_tracks ORDER BY pinned_at, track_id")
	if err != nil {
		return nil, fmt.Errorf("query pinned tracks: %w", err)
	}
	defer rows.Close()
	var tracks []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan pinned track: %w", err)
		}
		tracks = append(tracks, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pinned tracks: %w", err)
	}
	return tracks, nil
}

func (c *Catalog) TrackPinned(ctx context.Context, trackID string) (bool, error) {
	resolved, err := c.resolveTrackID(ctx, trackID)
	if err != nil {
		return false, err
	}
	var pinned bool
	if err := c.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pinned_tracks WHERE track_id = ?)", resolved,
	).Scan(&pinned); err != nil {
		return false, fmt.Errorf("query track pin: %w", err)
	}
	return pinned, nil
}
