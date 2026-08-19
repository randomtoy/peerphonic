package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func (c *Catalog) SaveTrackSource(ctx context.Context, source domain.TrackSource) error {
	return c.SaveTrackSources(ctx, []domain.TrackSource{source})
}

func (c *Catalog) SaveTrackSources(ctx context.Context, sources []domain.TrackSource) error {
	if len(sources) == 0 {
		return nil
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin track sources save: %w", err)
	}
	defer tx.Rollback()
	for _, source := range sources {
		if err := saveTrackSource(ctx, tx, source); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit track sources save: %w", err)
	}
	return nil
}

func (c *Catalog) SaveTrackAlias(ctx context.Context, aliasID, trackID string) error {
	if aliasID == "" || trackID == "" || aliasID == trackID {
		return nil
	}
	if _, err := c.db.ExecContext(ctx, `INSERT INTO track_aliases(alias_id, track_id)
		VALUES (?, ?) ON CONFLICT(alias_id) DO UPDATE SET track_id = excluded.track_id`,
		aliasID, trackID); err != nil {
		return fmt.Errorf("save track alias %q: %w", aliasID, err)
	}
	return nil
}

func saveTrackSource(ctx context.Context, tx *sql.Tx, source domain.TrackSource) error {
	if source.Track.ID == "" || source.Ref.Provider == "" || source.Ref.Key == "" {
		return fmt.Errorf("track id, provider, and source key are required")
	}
	track := source.Track
	albumArtistID := track.AlbumArtistID
	if albumArtistID == "" {
		albumArtistID = track.ArtistID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tracks (
		id, title, artist, artist_id, album, album_id,
		album_artist, album_artist_id, track_number, disc_number, year, genre, duration_ms, size_bytes,
		bit_rate, suffix, content_type, cover_art_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		title = excluded.title,
		artist = excluded.artist,
		artist_id = excluded.artist_id,
		album = excluded.album,
		album_id = excluded.album_id,
		album_artist = excluded.album_artist,
		album_artist_id = excluded.album_artist_id,
		track_number = excluded.track_number,
		disc_number = excluded.disc_number,
		year = excluded.year,
		genre = excluded.genre,
		duration_ms = excluded.duration_ms,
		size_bytes = excluded.size_bytes,
		bit_rate = excluded.bit_rate,
		suffix = excluded.suffix,
		content_type = excluded.content_type,
		cover_art_id = excluded.cover_art_id`,
		track.ID, track.Title, track.Artist, track.ArtistID, track.Album, track.AlbumID,
		track.AlbumArtist, albumArtistID, track.TrackNumber, track.DiscNumber, track.Year, track.Genre,
		track.Duration.Milliseconds(), track.Size, track.BitRate, track.Suffix, track.ContentType, track.CoverArtID,
	); err != nil {
		return fmt.Errorf("save track %q: %w", track.ID, err)
	}
	discoveredAt := ""
	if !source.DiscoveredAt.IsZero() {
		discoveredAt = source.DiscoveredAt.UTC().Format(time.RFC3339Nano)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO track_sources (
		track_id, provider, source_key, discovered_at
	) VALUES (?, ?, ?, ?)
	ON CONFLICT(provider, source_key) DO UPDATE SET
		track_id = excluded.track_id,
		discovered_at = excluded.discovered_at`,
		track.ID, source.Ref.Provider, source.Ref.Key, discoveredAt,
	); err != nil {
		return fmt.Errorf("save source for track %q: %w", track.ID, err)
	}
	return nil
}
