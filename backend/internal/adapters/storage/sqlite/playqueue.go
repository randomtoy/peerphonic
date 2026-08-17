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

func (c *Catalog) PlayQueue(ctx context.Context, owner string) (domain.PlayQueue, error) {
	queue := domain.PlayQueue{Owner: owner}
	var changedAt string
	err := c.db.QueryRowContext(ctx, `SELECT current_track_id, position_ms, changed_at, changed_by
		FROM play_queues WHERE owner = ?`, owner).Scan(
		&queue.CurrentID, &queue.PositionMS, &changedAt, &queue.ChangedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlayQueue{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.PlayQueue{}, fmt.Errorf("query play queue: %w", err)
	}
	queue.Changed, err = time.Parse(time.RFC3339Nano, changedAt)
	if err != nil {
		return domain.PlayQueue{}, fmt.Errorf("parse play queue changed time: %w", err)
	}

	rows, err := c.db.QueryContext(ctx, "SELECT "+trackColumns+` FROM tracks
		JOIN play_queue_tracks pqt ON pqt.track_id = tracks.id
		WHERE pqt.owner = ? ORDER BY pqt.position`, owner)
	if err != nil {
		return domain.PlayQueue{}, fmt.Errorf("query play queue tracks: %w", err)
	}
	defer rows.Close()
	queue.Tracks, err = scanTracks(rows, "play queue")
	if err != nil {
		return domain.PlayQueue{}, err
	}
	return queue, nil
}

func (c *Catalog) SavePlayQueue(ctx context.Context, queue domain.PlayQueue) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin play queue save: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO play_queues (
		owner, current_track_id, position_ms, changed_at, changed_by
	) VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(owner) DO UPDATE SET
		current_track_id = excluded.current_track_id,
		position_ms = excluded.position_ms,
		changed_at = excluded.changed_at,
		changed_by = excluded.changed_by`,
		queue.Owner, queue.CurrentID, queue.PositionMS,
		queue.Changed.UTC().Format(time.RFC3339Nano), queue.ChangedBy,
	); err != nil {
		return fmt.Errorf("save play queue: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM play_queue_tracks WHERE owner = ?", queue.Owner); err != nil {
		return fmt.Errorf("clear play queue tracks: %w", err)
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO play_queue_tracks (
		owner, position, track_id
	) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare play queue track insert: %w", err)
	}
	defer statement.Close()
	for position, track := range queue.Tracks {
		if _, err := statement.ExecContext(ctx, queue.Owner, position, track.ID); err != nil {
			return fmt.Errorf("insert play queue track %q: %w", track.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit play queue save: %w", err)
	}
	return nil
}
