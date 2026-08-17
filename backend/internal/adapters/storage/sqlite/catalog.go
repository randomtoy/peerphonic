package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/migrations"
	_ "modernc.org/sqlite"
)

type Catalog struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Catalog, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	catalog := &Catalog{db: db}
	if err := catalog.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := catalog.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return catalog, nil
}

func (c *Catalog) Close() error {
	return c.db.Close()
}

func (c *Catalog) configure(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := c.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	return nil
}

func (c *Catalog) migrate(ctx context.Context) error {
	if _, err := c.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if err := c.applyMigration(ctx, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (c *Catalog) applyMigration(ctx context.Context, name string) error {
	var applied bool
	if err := c.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", name,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check migration %s: %w", name, err)
	}
	if applied {
		return nil
	}

	contents, err := migrations.Files.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)",
		name, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}

func (c *Catalog) ReplaceProviderTracks(ctx context.Context, provider string, tracks []domain.Track) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin catalog replacement: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM tracks WHERE provider = ?", provider); err != nil {
		return fmt.Errorf("clear provider catalog: %w", err)
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO tracks (
		id, provider, source_key, title, artist, artist_id, album, album_id,
		album_artist, album_artist_id, track_number, disc_number, year, duration_ms, size_bytes,
		bit_rate, suffix, content_type, cover_art_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare track insert: %w", err)
	}
	defer statement.Close()

	for _, track := range tracks {
		if track.Source.Provider != provider {
			return fmt.Errorf("track %q belongs to provider %q, want %q", track.ID, track.Source.Provider, provider)
		}
		albumArtistID := track.AlbumArtistID
		if albumArtistID == "" {
			albumArtistID = track.ArtistID
		}
		if _, err := statement.ExecContext(ctx,
			track.ID, track.Source.Provider, track.Source.Key, track.Title, track.Artist,
			track.ArtistID, track.Album, track.AlbumID, track.AlbumArtist, albumArtistID,
			track.TrackNumber, track.DiscNumber, track.Year, track.Duration.Milliseconds(),
			track.Size, track.BitRate, track.Suffix, track.ContentType, track.CoverArtID,
		); err != nil {
			return fmt.Errorf("insert track %q: %w", track.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit catalog replacement: %w", err)
	}
	return nil
}

const trackColumns = `id, provider, source_key, title, artist, artist_id, album,
	album_id, album_artist, album_artist_id, track_number, disc_number, year, duration_ms,
	size_bytes, bit_rate, suffix, content_type, cover_art_id`

func (c *Catalog) Track(ctx context.Context, id string) (domain.Track, error) {
	row := c.db.QueryRowContext(ctx, "SELECT "+trackColumns+" FROM tracks WHERE id = ?", id)
	track, err := scanTrack(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Track{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Track{}, fmt.Errorf("query track: %w", err)
	}
	return track, nil
}

func (c *Catalog) Artists(ctx context.Context) ([]domain.Artist, error) {
	rows, err := c.db.QueryContext(ctx, `WITH artist_roles(id, name, album_id) AS (
		SELECT album_artist_id, album_artist, album_id FROM tracks
		UNION ALL
		SELECT artist_id, artist, album_id FROM tracks
	)
	SELECT id, MIN(name), COUNT(DISTINCT album_id) FROM artist_roles
	GROUP BY id ORDER BY MIN(name) COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("query artists: %w", err)
	}
	defer rows.Close()

	var artists []domain.Artist
	for rows.Next() {
		var artist domain.Artist
		if err := rows.Scan(&artist.ID, &artist.Name, &artist.AlbumCount); err != nil {
			return nil, fmt.Errorf("scan artist: %w", err)
		}
		artists = append(artists, artist)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artists: %w", err)
	}
	return artists, nil
}

func (c *Catalog) Artist(ctx context.Context, id string) (domain.Artist, error) {
	row := c.db.QueryRowContext(ctx, `WITH artist_roles(id, name, album_id) AS (
		SELECT album_artist_id, album_artist, album_id FROM tracks
		UNION ALL
		SELECT artist_id, artist, album_id FROM tracks
	)
	SELECT id, MIN(name), COUNT(DISTINCT album_id) FROM artist_roles
	WHERE id = ? GROUP BY id`, id)
	var artist domain.Artist
	if err := row.Scan(&artist.ID, &artist.Name, &artist.AlbumCount); errors.Is(err, sql.ErrNoRows) {
		return domain.Artist{}, ports.ErrNotFound
	} else if err != nil {
		return domain.Artist{}, fmt.Errorf("query artist: %w", err)
	}
	return artist, nil
}

func (c *Catalog) Albums(ctx context.Context, offset, limit int) ([]domain.Album, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT album_id, album, album_artist, album_artist_id,
		MIN(NULLIF(year, 0)), COUNT(*), SUM(duration_ms), MIN(NULLIF(cover_art_id, ''))
		FROM tracks GROUP BY album_id, album, album_artist, album_artist_id
		ORDER BY album COLLATE NOCASE, album_artist COLLATE NOCASE LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query all albums: %w", err)
	}
	defer rows.Close()
	return scanAlbums(rows)
}

func (c *Catalog) AlbumsByArtist(ctx context.Context, artistID string) ([]domain.Album, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT album_id, album, album_artist, album_artist_id,
		MIN(NULLIF(year, 0)), COUNT(*), SUM(duration_ms), MIN(NULLIF(cover_art_id, ''))
		FROM tracks WHERE album_id IN (
			SELECT DISTINCT album_id FROM tracks WHERE album_artist_id = ? OR artist_id = ?
		)
		GROUP BY album_id, album, album_artist, album_artist_id
		ORDER BY album COLLATE NOCASE`, artistID, artistID)
	if err != nil {
		return nil, fmt.Errorf("query albums: %w", err)
	}
	defer rows.Close()

	return scanAlbums(rows)
}

func scanAlbums(rows *sql.Rows) ([]domain.Album, error) {
	var albums []domain.Album
	for rows.Next() {
		var album domain.Album
		var year sql.NullInt64
		var coverArtID sql.NullString
		var durationMS int64
		if err := rows.Scan(&album.ID, &album.Name, &album.Artist, &album.ArtistID,
			&year, &album.SongCount, &durationMS, &coverArtID); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		album.Year = int(year.Int64)
		album.Duration = time.Duration(durationMS) * time.Millisecond
		album.CoverArtID = coverArtID.String
		albums = append(albums, album)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate albums: %w", err)
	}
	return albums, nil
}

func (c *Catalog) TracksByAlbum(ctx context.Context, albumID string) ([]domain.Track, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT "+trackColumns+` FROM tracks WHERE album_id = ?
		ORDER BY disc_number, track_number, title COLLATE NOCASE`, albumID)
	if err != nil {
		return nil, fmt.Errorf("query album tracks: %w", err)
	}
	defer rows.Close()

	var tracks []domain.Track
	for rows.Next() {
		track, err := scanTrack(rows)
		if err != nil {
			return nil, fmt.Errorf("scan album track: %w", err)
		}
		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate album tracks: %w", err)
	}
	return tracks, nil
}

func (c *Catalog) Search(ctx context.Context, query ports.CatalogSearch) (ports.CatalogSearchResult, error) {
	artists, err := c.Artists(ctx)
	if err != nil {
		return ports.CatalogSearchResult{}, err
	}
	allResults := int(^uint(0) >> 1)
	albums, err := c.Albums(ctx, 0, allResults)
	if err != nil {
		return ports.CatalogSearchResult{}, err
	}
	songs, err := c.allTracks(ctx)
	if err != nil {
		return ports.CatalogSearchResult{}, err
	}

	needle := strings.ToLower(strings.TrimSpace(query.Text))
	matchingArtists := filter(artists, func(artist domain.Artist) bool {
		return containsFolded(artist.Name, needle)
	})
	matchingAlbums := filter(albums, func(album domain.Album) bool {
		return containsFolded(album.Name, needle) || containsFolded(album.Artist, needle)
	})
	matchingSongs := filter(songs, func(song domain.Track) bool {
		return containsFolded(song.Title, needle) || containsFolded(song.Artist, needle) ||
			containsFolded(song.Album, needle)
	})

	return ports.CatalogSearchResult{
		Artists: page(matchingArtists, query.ArtistOffset, query.ArtistCount),
		Albums:  page(matchingAlbums, query.AlbumOffset, query.AlbumCount),
		Songs:   page(matchingSongs, query.SongOffset, query.SongCount),
	}, nil
}

func (c *Catalog) allTracks(ctx context.Context) ([]domain.Track, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT "+trackColumns+
		" FROM tracks ORDER BY title COLLATE NOCASE, artist COLLATE NOCASE")
	if err != nil {
		return nil, fmt.Errorf("query all tracks: %w", err)
	}
	defer rows.Close()

	var tracks []domain.Track
	for rows.Next() {
		track, err := scanTrack(rows)
		if err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tracks: %w", err)
	}
	return tracks, nil
}

func containsFolded(value, foldedNeedle string) bool {
	return foldedNeedle == "" || strings.Contains(strings.ToLower(value), foldedNeedle)
}

func filter[T any](items []T, keep func(T) bool) []T {
	result := make([]T, 0, len(items))
	for _, item := range items {
		if keep(item) {
			result = append(result, item)
		}
	}
	return result
}

func page[T any](items []T, offset, count int) []T {
	if offset < 0 || count <= 0 || offset >= len(items) {
		return []T{}
	}
	end := offset + count
	if end < offset || end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTrack(row rowScanner) (domain.Track, error) {
	var track domain.Track
	var durationMS int64
	err := row.Scan(
		&track.ID, &track.Source.Provider, &track.Source.Key, &track.Title, &track.Artist,
		&track.ArtistID, &track.Album, &track.AlbumID, &track.AlbumArtist,
		&track.AlbumArtistID,
		&track.TrackNumber, &track.DiscNumber, &track.Year, &durationMS,
		&track.Size, &track.BitRate, &track.Suffix, &track.ContentType, &track.CoverArtID,
	)
	track.Duration = time.Duration(durationMS) * time.Millisecond
	return track, err
}
