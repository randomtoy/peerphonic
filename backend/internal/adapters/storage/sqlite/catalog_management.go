package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func (c *Catalog) resolveArtistID(ctx context.Context, id string) (string, error) {
	var resolved string
	err := c.db.QueryRowContext(ctx, `WITH artist_roles(id) AS (
		SELECT artist_id FROM tracks UNION SELECT album_artist_id FROM tracks
	) SELECT COALESCE((SELECT target_id FROM artist_aliases WHERE alias_id = ?), ?)
	WHERE EXISTS (SELECT 1 FROM artist_roles WHERE id = ?)
		OR EXISTS (SELECT 1 FROM artist_aliases WHERE alias_id = ? OR target_id = ?)`,
		id, id, id, id, id).Scan(&resolved)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ports.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve artist ID: %w", err)
	}
	return resolved, nil
}

func (c *Catalog) ArtistAliases(ctx context.Context) ([]domain.ArtistAlias, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT alias_id, alias_name, target_id, target_name,
		created_at FROM artist_aliases ORDER BY LOWER(alias_name)`)
	if err != nil {
		return nil, fmt.Errorf("query artist aliases: %w", err)
	}
	defer rows.Close()
	var aliases []domain.ArtistAlias
	for rows.Next() {
		var alias domain.ArtistAlias
		var createdAt string
		if err := rows.Scan(&alias.AliasID, &alias.AliasName, &alias.TargetID,
			&alias.TargetName, &createdAt); err != nil {
			return nil, fmt.Errorf("scan artist alias: %w", err)
		}
		alias.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse artist alias time: %w", err)
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artist aliases: %w", err)
	}
	return aliases, nil
}

func (c *Catalog) SetArtistAlias(
	ctx context.Context, aliasID, targetID string,
) (domain.ArtistAlias, error) {
	aliasID, targetID = strings.TrimSpace(aliasID), strings.TrimSpace(targetID)
	if aliasID == "" || targetID == "" || aliasID == targetID {
		return domain.ArtistAlias{}, fmt.Errorf("artist alias and target must be different")
	}
	alias, err := c.Artist(ctx, aliasID)
	if err != nil {
		return domain.ArtistAlias{}, err
	}
	targetID, err = c.resolveArtistID(ctx, targetID)
	if err != nil {
		return domain.ArtistAlias{}, err
	}
	if alias.ID == targetID {
		return domain.ArtistAlias{}, fmt.Errorf("artist alias cycle")
	}
	target, err := c.Artist(ctx, targetID)
	if err != nil {
		return domain.ArtistAlias{}, err
	}
	createdAt := time.Now().UTC()
	if _, err := c.db.ExecContext(ctx, `INSERT INTO artist_aliases(
		alias_id, alias_name, target_id, target_name, created_at
	) VALUES (?, ?, ?, ?, ?) ON CONFLICT(alias_id) DO UPDATE SET
		target_id=excluded.target_id, target_name=excluded.target_name`,
		aliasID, alias.Name, target.ID, target.Name, createdAt.Format(time.RFC3339Nano)); err != nil {
		return domain.ArtistAlias{}, fmt.Errorf("save artist alias: %w", err)
	}
	return domain.ArtistAlias{
		AliasID: aliasID, AliasName: alias.Name, TargetID: target.ID,
		TargetName: target.Name, CreatedAt: createdAt,
	}, nil
}

func (c *Catalog) DeleteArtistAlias(ctx context.Context, aliasID string) error {
	result, err := c.db.ExecContext(ctx, "DELETE FROM artist_aliases WHERE alias_id = ?", aliasID)
	if err != nil {
		return fmt.Errorf("delete artist alias: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted artist aliases: %w", err)
	}
	if deleted == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (c *Catalog) UpdateTrackMetadata(
	ctx context.Context, id string, patch domain.TrackMetadataPatch,
) (domain.Track, error) {
	track, err := c.Track(ctx, id)
	if err != nil {
		return domain.Track{}, err
	}
	applyText := func(target *string, value *string) error {
		if value == nil {
			return nil
		}
		clean := strings.TrimSpace(*value)
		if clean == "" {
			return fmt.Errorf("metadata values cannot be empty")
		}
		*target = clean
		return nil
	}
	for target, value := range map[*string]*string{
		&track.Title: patch.Title, &track.Artist: patch.Artist, &track.Album: patch.Album,
		&track.AlbumArtist: patch.AlbumArtist,
	} {
		if err := applyText(target, value); err != nil {
			return domain.Track{}, err
		}
	}
	if patch.Genre != nil {
		track.Genre = strings.TrimSpace(*patch.Genre)
	}
	for target, value := range map[*int]*int{
		&track.Year: patch.Year, &track.TrackNumber: patch.TrackNumber, &track.DiscNumber: patch.DiscNumber,
	} {
		if value != nil {
			if *value < 0 {
				return domain.Track{}, fmt.Errorf("metadata numbers cannot be negative")
			}
			*target = *value
		}
	}
	track.ArtistID = domain.CanonicalArtistID(track.Artist)
	if track.AlbumArtist == "" {
		track.AlbumArtist = track.Artist
	}
	track.AlbumArtistID = domain.CanonicalArtistID(track.AlbumArtist)
	track.AlbumID = domain.CanonicalAlbumID(track.AlbumArtist, track.Album)
	if err := c.UpdateTrack(ctx, track); err != nil {
		return domain.Track{}, err
	}
	return track, nil
}
