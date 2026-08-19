package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func (c *Catalog) MediaAnnotations(ctx context.Context, owner string) ([]domain.MediaAnnotation, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT media_type, media_id, starred_at,
		rating, play_count, last_played_at FROM media_annotations
		WHERE owner = ? ORDER BY starred_at DESC, media_type, media_id`, owner)
	if err != nil {
		return nil, fmt.Errorf("query media annotations: %w", err)
	}
	defer rows.Close()

	var annotations []domain.MediaAnnotation
	for rows.Next() {
		annotation := domain.MediaAnnotation{Owner: owner}
		var starredAt, lastPlayedAt string
		if err := rows.Scan(
			&annotation.Media.Type, &annotation.Media.ID, &starredAt,
			&annotation.Rating, &annotation.PlayCount, &lastPlayedAt,
		); err != nil {
			return nil, fmt.Errorf("scan media annotation: %w", err)
		}
		annotation.StarredAt, err = parseOptionalTime(starredAt)
		if err != nil {
			return nil, fmt.Errorf("parse starred time: %w", err)
		}
		annotation.LastPlayed, err = parseOptionalTime(lastPlayedAt)
		if err != nil {
			return nil, fmt.Errorf("parse last played time: %w", err)
		}
		annotations = append(annotations, annotation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media annotations: %w", err)
	}
	return annotations, nil
}

func (c *Catalog) UpdateMediaAnnotations(ctx context.Context, updates []ports.MediaAnnotationUpdate) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin media annotation save: %w", err)
	}
	defer tx.Rollback()
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_annotations (
			owner, media_type, media_id
		) VALUES (?, ?, ?) ON CONFLICT(owner, media_type, media_id) DO NOTHING`,
			update.Owner, update.Media.Type, update.Media.ID); err != nil {
			return fmt.Errorf("create media annotation %q: %w", update.Media.ID, err)
		}
		starredAt, setStarred := optionalTimeUpdate(update.StarredAt)
		rating, setRating := optionalIntUpdate(update.Rating)
		lastPlayed, setLastPlayed := optionalTimeUpdate(update.LastPlayed)
		if _, err := tx.ExecContext(ctx, `UPDATE media_annotations SET
			starred_at = CASE WHEN ? THEN ? ELSE starred_at END,
			rating = CASE WHEN ? THEN ? ELSE rating END,
			play_count = play_count + ?,
			last_played_at = CASE WHEN ? THEN ? ELSE last_played_at END
			WHERE owner = ? AND media_type = ? AND media_id = ?`,
			setStarred, starredAt, setRating, rating, update.PlayCountDelta,
			setLastPlayed, lastPlayed, update.Owner, update.Media.Type, update.Media.ID,
		); err != nil {
			return fmt.Errorf("update media annotation %q: %w", update.Media.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_annotations
			WHERE owner = ? AND media_type = ? AND media_id = ?
				AND starred_at = '' AND rating = 0 AND play_count = 0 AND last_played_at = ''`,
			update.Owner, update.Media.Type, update.Media.ID); err != nil {
			return fmt.Errorf("remove empty media annotation %q: %w", update.Media.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit media annotations: %w", err)
	}
	return nil
}

func parseOptionalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func optionalTimeUpdate(value *time.Time) (string, bool) {
	if value == nil {
		return "", false
	}
	return formatOptionalTime(*value), true
}

func optionalIntUpdate(value *int) (int, bool) {
	if value == nil {
		return 0, false
	}
	return *value, true
}
