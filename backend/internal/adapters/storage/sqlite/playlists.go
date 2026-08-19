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

func (c *Catalog) Playlists(ctx context.Context, owner string) ([]domain.Playlist, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT p.id, p.name, p.comment, p.owner, p.public,
		p.created_at, p.changed_at, COUNT(t.id), COALESCE(SUM(t.duration_ms), 0)
		FROM playlists p
		LEFT JOIN playlist_tracks pt ON pt.playlist_id = p.id
		LEFT JOIN tracks t ON t.id = pt.track_id
		WHERE p.owner = ? OR p.public = TRUE
		GROUP BY p.id ORDER BY LOWER(p.name)`, owner)
	if err != nil {
		return nil, fmt.Errorf("query playlists: %w", err)
	}
	defer rows.Close()

	var playlists []domain.Playlist
	for rows.Next() {
		playlist, err := scanPlaylistSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("scan playlist: %w", err)
		}
		playlists = append(playlists, playlist)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate playlists: %w", err)
	}
	return playlists, nil
}

func (c *Catalog) Playlist(ctx context.Context, id string) (domain.Playlist, error) {
	row := c.db.QueryRowContext(ctx, `SELECT id, name, comment, owner, public,
		created_at, changed_at FROM playlists WHERE id = ?`, id)
	var playlist domain.Playlist
	var createdAt, changedAt string
	if err := row.Scan(
		&playlist.ID, &playlist.Name, &playlist.Comment, &playlist.Owner, &playlist.Public,
		&createdAt, &changedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return domain.Playlist{}, ports.ErrNotFound
	} else if err != nil {
		return domain.Playlist{}, fmt.Errorf("query playlist: %w", err)
	}
	var err error
	playlist.Created, err = parsePlaylistTime(createdAt)
	if err != nil {
		return domain.Playlist{}, err
	}
	playlist.Changed, err = parsePlaylistTime(changedAt)
	if err != nil {
		return domain.Playlist{}, err
	}

	rows, err := c.db.QueryContext(ctx, "SELECT "+trackColumns+` FROM tracks
		JOIN playlist_tracks pt ON pt.track_id = tracks.id
		WHERE pt.playlist_id = ? ORDER BY pt.position`, id)
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("query playlist tracks: %w", err)
	}
	defer rows.Close()
	playlist.Tracks, err = scanTracks(rows, "playlist")
	if err != nil {
		return domain.Playlist{}, err
	}
	playlist.SongCount = len(playlist.Tracks)
	for _, track := range playlist.Tracks {
		playlist.Duration += track.Duration
	}
	return playlist, nil
}

func (c *Catalog) SavePlaylist(ctx context.Context, playlist domain.Playlist) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin playlist save: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO playlists (
		id, owner, name, comment, public, created_at, changed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		owner = excluded.owner, name = excluded.name, comment = excluded.comment,
		public = excluded.public, changed_at = excluded.changed_at`,
		playlist.ID, playlist.Owner, playlist.Name, playlist.Comment, playlist.Public,
		playlist.Created.UTC().Format(time.RFC3339Nano), playlist.Changed.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("save playlist: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM playlist_tracks WHERE playlist_id = ?", playlist.ID); err != nil {
		return fmt.Errorf("clear playlist tracks: %w", err)
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO playlist_tracks (
		playlist_id, position, track_id
	) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare playlist track insert: %w", err)
	}
	defer statement.Close()
	for position, track := range playlist.Tracks {
		if _, err := statement.ExecContext(ctx, playlist.ID, position, track.ID); err != nil {
			return fmt.Errorf("insert playlist track %q: %w", track.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit playlist save: %w", err)
	}
	return nil
}

func (c *Catalog) DeletePlaylist(ctx context.Context, id string) error {
	result, err := c.db.ExecContext(ctx, "DELETE FROM playlists WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete playlist: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted playlists: %w", err)
	}
	if deleted == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func scanPlaylistSummary(row rowScanner) (domain.Playlist, error) {
	var playlist domain.Playlist
	var createdAt, changedAt string
	var durationMS int64
	if err := row.Scan(
		&playlist.ID, &playlist.Name, &playlist.Comment, &playlist.Owner, &playlist.Public,
		&createdAt, &changedAt, &playlist.SongCount, &durationMS,
	); err != nil {
		return domain.Playlist{}, err
	}
	var err error
	playlist.Created, err = parsePlaylistTime(createdAt)
	if err != nil {
		return domain.Playlist{}, err
	}
	playlist.Changed, err = parsePlaylistTime(changedAt)
	if err != nil {
		return domain.Playlist{}, err
	}
	playlist.Duration = time.Duration(durationMS) * time.Millisecond
	return playlist, nil
}

func parsePlaylistTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse playlist time: %w", err)
	}
	return parsed, nil
}
