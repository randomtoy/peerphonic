package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

const canonicalCatalogIdentityMigration = "data_001_canonical_catalog_identities"

type identityUpdate struct {
	trackID       string
	artistID      string
	albumID       string
	albumArtistID string
}

type storedAnnotation struct {
	owner        string
	mediaType    string
	mediaID      string
	starredAt    string
	rating       int
	playCount    int64
	lastPlayedAt string
}

// migrateCanonicalCatalogIdentities repairs IDs written by older providers,
// which derived artist and album identity differently for the same names.
func (c *Catalog) migrateCanonicalCatalogIdentities(ctx context.Context) error {
	var applied bool
	if err := c.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)",
		canonicalCatalogIdentityMigration,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check canonical catalog identity migration: %w", err)
	}
	if applied {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin canonical catalog identity migration: %w", err)
	}
	defer tx.Rollback()

	updates, artistIDs, albumIDs, err := catalogIdentityUpdates(ctx, tx)
	if err != nil {
		return err
	}
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE tracks SET artist_id = ?, album_id = ?,
			album_artist_id = ? WHERE id = ?`, update.artistID, update.albumID,
			update.albumArtistID, update.trackID); err != nil {
			return fmt.Errorf("normalize identity for track %q: %w", update.trackID, err)
		}
	}
	if err := migrateIdentityAnnotations(ctx, tx, artistIDs, albumIDs); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)",
		canonicalCatalogIdentityMigration, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("record canonical catalog identity migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit canonical catalog identity migration: %w", err)
	}
	return nil
}

func catalogIdentityUpdates(
	ctx context.Context,
	tx *sql.Tx,
) ([]identityUpdate, map[string]string, map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, artist, artist_id, album, album_id,
		album_artist, album_artist_id FROM tracks`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query legacy catalog identities: %w", err)
	}
	defer rows.Close()

	var updates []identityUpdate
	artistIDs := make(map[string]string)
	albumIDs := make(map[string]string)
	for rows.Next() {
		var trackID, artist, oldArtistID, album, oldAlbumID, albumArtist, oldAlbumArtistID string
		if err := rows.Scan(&trackID, &artist, &oldArtistID, &album, &oldAlbumID,
			&albumArtist, &oldAlbumArtistID); err != nil {
			return nil, nil, nil, fmt.Errorf("scan legacy catalog identity: %w", err)
		}
		if albumArtist == "" {
			albumArtist = artist
		}
		artistID := domain.CanonicalArtistID(artist)
		albumArtistID := domain.CanonicalArtistID(albumArtist)
		albumID := domain.CanonicalAlbumID(albumArtist, album)
		updates = append(updates, identityUpdate{
			trackID: trackID, artistID: artistID, albumID: albumID, albumArtistID: albumArtistID,
		})
		addIdentityMapping(artistIDs, oldArtistID, artistID)
		addIdentityMapping(artistIDs, oldAlbumArtistID, albumArtistID)
		addIdentityMapping(albumIDs, oldAlbumID, albumID)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("iterate legacy catalog identities: %w", err)
	}
	return updates, artistIDs, albumIDs, nil
}

// An empty mapped value marks an ambiguous legacy ID and leaves its annotation untouched.
func addIdentityMapping(mappings map[string]string, oldID, newID string) {
	if oldID == "" {
		return
	}
	if current, exists := mappings[oldID]; exists && current != newID {
		mappings[oldID] = ""
		return
	}
	if _, exists := mappings[oldID]; !exists {
		mappings[oldID] = newID
	}
}

func migrateIdentityAnnotations(
	ctx context.Context,
	tx *sql.Tx,
	artistIDs map[string]string,
	albumIDs map[string]string,
) error {
	rows, err := tx.QueryContext(ctx, `SELECT owner, media_type, media_id, starred_at,
		rating, play_count, last_played_at FROM media_annotations
		WHERE media_type IN ('artist', 'album')`)
	if err != nil {
		return fmt.Errorf("query identity annotations: %w", err)
	}
	var annotations []storedAnnotation
	for rows.Next() {
		var annotation storedAnnotation
		if err := rows.Scan(&annotation.owner, &annotation.mediaType, &annotation.mediaID,
			&annotation.starredAt, &annotation.rating, &annotation.playCount,
			&annotation.lastPlayedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan identity annotation: %w", err)
		}
		annotations = append(annotations, annotation)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close identity annotation rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate identity annotations: %w", err)
	}

	merged := make(map[string]storedAnnotation, len(annotations))
	for _, annotation := range annotations {
		mappings := artistIDs
		if annotation.mediaType == string(domain.MediaAlbum) {
			mappings = albumIDs
		}
		if replacement := mappings[annotation.mediaID]; replacement != "" {
			annotation.mediaID = replacement
		}
		key := annotation.owner + "\x00" + annotation.mediaType + "\x00" + annotation.mediaID
		if current, exists := merged[key]; exists {
			current.starredAt = max(current.starredAt, annotation.starredAt)
			current.rating = max(current.rating, annotation.rating)
			current.playCount += annotation.playCount
			current.lastPlayedAt = max(current.lastPlayedAt, annotation.lastPlayedAt)
			merged[key] = current
		} else {
			merged[key] = annotation
		}
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM media_annotations WHERE media_type IN ('artist', 'album')"); err != nil {
		return fmt.Errorf("remove legacy identity annotations: %w", err)
	}
	for _, annotation := range merged {
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_annotations (
			owner, media_type, media_id, starred_at, rating, play_count, last_played_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`, annotation.owner, annotation.mediaType,
			annotation.mediaID, annotation.starredAt, annotation.rating,
			annotation.playCount, annotation.lastPlayedAt); err != nil {
			return fmt.Errorf("save canonical identity annotation: %w", err)
		}
	}
	return nil
}
