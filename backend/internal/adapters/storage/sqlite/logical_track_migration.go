package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

const logicalTrackMigration = "data_002_logical_tracks"

func (c *Catalog) migrateLogicalTracks(ctx context.Context) error {
	var applied bool
	if err := c.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", logicalTrackMigration,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check logical track migration: %w", err)
	}
	if applied {
		return nil
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin logical track migration: %w", err)
	}
	defer tx.Rollback()

	tracks, err := migrationTracks(ctx, tx)
	if err != nil {
		return err
	}
	ranks, err := migrationTrackSourceRanks(ctx, tx)
	if err != nil {
		return err
	}
	groups := make(map[string][]domain.Track)
	for _, track := range tracks {
		logicalID := track.ID
		if domain.HasCanonicalTrackIdentity(track) {
			logicalID = domain.CanonicalTrackID(track)
		}
		groups[logicalID] = append(groups[logicalID], track)
	}
	aliases := make(map[string]string, len(tracks))
	for logicalID, group := range groups {
		track := preferredMigrationTrack(group, ranks)
		track.ID = logicalID
		if err := upsertMigrationTrack(ctx, tx, track); err != nil {
			return err
		}
		for _, previous := range group {
			if previous.ID == logicalID {
				continue
			}
			aliases[previous.ID] = logicalID
			if err := moveTrackReferences(ctx, tx, previous.ID, logicalID); err != nil {
				return err
			}
		}
	}
	if err := migrateTrackAnnotations(ctx, tx, aliases); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)",
		logicalTrackMigration, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("record logical track migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit logical track migration: %w", err)
	}
	return nil
}

func migrationTracks(ctx context.Context, tx *transaction) ([]domain.Track, error) {
	rows, err := tx.QueryContext(ctx, "SELECT "+trackColumns+" FROM tracks")
	if err != nil {
		return nil, fmt.Errorf("query tracks for logical migration: %w", err)
	}
	defer rows.Close()
	var tracks []domain.Track
	for rows.Next() {
		track, err := scanTrack(rows)
		if err != nil {
			return nil, fmt.Errorf("scan track for logical migration: %w", err)
		}
		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tracks for logical migration: %w", err)
	}
	return tracks, nil
}

func migrationTrackSourceRanks(ctx context.Context, tx *transaction) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT track_id, MIN(CASE provider
		WHEN 'local' THEN 0 WHEN 'torrent' THEN 1 WHEN 'soulseek' THEN 2 ELSE 3 END)
		FROM track_sources GROUP BY track_id`)
	if err != nil {
		return nil, fmt.Errorf("query track source ranks: %w", err)
	}
	defer rows.Close()
	ranks := make(map[string]int)
	for rows.Next() {
		var rank int
		var trackID string
		if err := rows.Scan(&trackID, &rank); err != nil {
			return nil, fmt.Errorf("scan track source rank: %w", err)
		}
		ranks[trackID] = rank
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate track source ranks: %w", err)
	}
	return ranks, nil
}

func preferredMigrationTrack(tracks []domain.Track, ranks map[string]int) domain.Track {
	best := tracks[0]
	bestScore := migrationTrackScore(best, ranks[best.ID])
	for _, candidate := range tracks[1:] {
		if score := migrationTrackScore(candidate, ranks[candidate.ID]); score > bestScore {
			best, bestScore = candidate, score
		}
	}
	for _, candidate := range tracks {
		if best.Genre == "" {
			best.Genre = candidate.Genre
		}
		if best.CoverArtID == "" {
			best.CoverArtID = candidate.CoverArtID
		}
		if best.Year == 0 {
			best.Year = candidate.Year
		}
		if best.Duration == 0 {
			best.Duration = candidate.Duration
		}
	}
	return best
}

func migrationTrackScore(track domain.Track, sourceRank int) int {
	score := (4 - sourceRank) * 100
	if track.Duration > 0 {
		score += 10
	}
	if track.CoverArtID != "" {
		score += 4
	}
	if track.Genre != "" {
		score += 2
	}
	if track.Year != 0 {
		score++
	}
	return score
}

func upsertMigrationTrack(ctx context.Context, tx *transaction, track domain.Track) error {
	albumArtistID := track.AlbumArtistID
	if albumArtistID == "" {
		albumArtistID = track.ArtistID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO tracks (
		id, title, artist, artist_id, album, album_id, album_artist, album_artist_id,
		track_number, disc_number, year, genre, duration_ms, size_bytes, bit_rate,
		suffix, content_type, cover_art_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET title=excluded.title, artist=excluded.artist,
		artist_id=excluded.artist_id, album=excluded.album, album_id=excluded.album_id,
		album_artist=excluded.album_artist, album_artist_id=excluded.album_artist_id,
		track_number=excluded.track_number, disc_number=excluded.disc_number,
		year=excluded.year, genre=excluded.genre, duration_ms=excluded.duration_ms,
		size_bytes=excluded.size_bytes, bit_rate=excluded.bit_rate, suffix=excluded.suffix,
		content_type=excluded.content_type, cover_art_id=excluded.cover_art_id`,
		track.ID, track.Title, track.Artist, track.ArtistID, track.Album, track.AlbumID,
		track.AlbumArtist, albumArtistID, track.TrackNumber, track.DiscNumber, track.Year,
		track.Genre, track.Duration.Milliseconds(), track.Size, track.BitRate, track.Suffix,
		track.ContentType, track.CoverArtID,
	)
	if err != nil {
		return fmt.Errorf("save logical track %q: %w", track.ID, err)
	}
	return nil
}

func moveTrackReferences(ctx context.Context, tx *transaction, oldID, newID string) error {
	statements := []struct {
		query string
		args  []any
	}{
		{`UPDATE track_sources SET track_id = ? WHERE track_id = ?`, []any{newID, oldID}},
		{`INSERT OR IGNORE INTO album_alias_tracks(provider, alias_id, track_id)
			SELECT provider, alias_id, ? FROM album_alias_tracks WHERE track_id = ?`, []any{newID, oldID}},
		{`DELETE FROM album_alias_tracks WHERE track_id = ?`, []any{oldID}},
		{`UPDATE playlist_tracks SET track_id = ? WHERE track_id = ?`, []any{newID, oldID}},
		{`UPDATE play_queue_tracks SET track_id = ? WHERE track_id = ?`, []any{newID, oldID}},
		{`UPDATE play_queues SET current_track_id = ? WHERE current_track_id = ?`, []any{newID, oldID}},
		{`UPDATE track_aliases SET track_id = ? WHERE track_id = ?`, []any{newID, oldID}},
		{`INSERT OR REPLACE INTO track_aliases(alias_id, track_id) VALUES (?, ?)`, []any{oldID, newID}},
		{`DELETE FROM tracks WHERE id = ?`, []any{oldID}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("move track reference %q to %q: %w", oldID, newID, err)
		}
	}
	return nil
}

func migrateTrackAnnotations(ctx context.Context, tx *transaction, aliases map[string]string) error {
	rows, err := tx.QueryContext(ctx, `SELECT owner, media_type, media_id, starred_at,
		rating, play_count, last_played_at FROM media_annotations WHERE media_type = 'song'`)
	if err != nil {
		return fmt.Errorf("query track annotations: %w", err)
	}
	var annotations []storedAnnotation
	for rows.Next() {
		var annotation storedAnnotation
		if err := rows.Scan(&annotation.owner, &annotation.mediaType, &annotation.mediaID,
			&annotation.starredAt, &annotation.rating, &annotation.playCount,
			&annotation.lastPlayedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan track annotation: %w", err)
		}
		annotations = append(annotations, annotation)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close track annotation rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate track annotations: %w", err)
	}
	merged := make(map[string]storedAnnotation, len(annotations))
	for _, annotation := range annotations {
		if replacement := aliases[annotation.mediaID]; replacement != "" {
			annotation.mediaID = replacement
		}
		key := annotation.owner + "\x00" + annotation.mediaID
		if current, ok := merged[key]; ok {
			current.starredAt = max(current.starredAt, annotation.starredAt)
			current.rating = max(current.rating, annotation.rating)
			current.playCount += annotation.playCount
			current.lastPlayedAt = max(current.lastPlayedAt, annotation.lastPlayedAt)
			merged[key] = current
		} else {
			merged[key] = annotation
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM media_annotations WHERE media_type = 'song'"); err != nil {
		return fmt.Errorf("remove legacy track annotations: %w", err)
	}
	for _, annotation := range merged {
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_annotations (
			owner, media_type, media_id, starred_at, rating, play_count, last_played_at
		) VALUES (?, 'song', ?, ?, ?, ?, ?)`, annotation.owner, annotation.mediaID,
			annotation.starredAt, annotation.rating, annotation.playCount,
			annotation.lastPlayedAt); err != nil {
			return fmt.Errorf("save logical track annotation: %w", err)
		}
	}
	return nil
}
