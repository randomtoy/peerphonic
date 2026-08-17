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

func (c *Catalog) ReplaceProviderTracks(
	ctx context.Context,
	provider string,
	sources []domain.TrackSource,
	albumAliases []ports.AlbumAlias,
) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin catalog replacement: %w", err)
	}
	defer tx.Rollback()
	previousAlbums := make(map[string]string, len(sources))
	previousAliases := make(map[string][]string, len(sources))
	incomingTrackIDs := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		incomingTrackIDs[source.Track.ID] = struct{}{}
		var albumID string
		err := tx.QueryRowContext(ctx, "SELECT album_id FROM tracks WHERE id = ?", source.Track.ID).Scan(&albumID)
		if err == nil {
			previousAlbums[source.Track.ID] = albumID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("query previous album for track %q: %w", source.Track.ID, err)
		}
		rows, err := tx.QueryContext(ctx, `SELECT alias_id FROM album_alias_tracks
			WHERE provider = ? AND track_id = ?`, provider, source.Track.ID)
		if err != nil {
			return fmt.Errorf("query previous aliases for track %q: %w", source.Track.ID, err)
		}
		for rows.Next() {
			var aliasID string
			if err := rows.Scan(&aliasID); err != nil {
				rows.Close()
				return fmt.Errorf("scan previous alias for track %q: %w", source.Track.ID, err)
			}
			previousAliases[source.Track.ID] = append(previousAliases[source.Track.ID], aliasID)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close previous aliases for track %q: %w", source.Track.ID, err)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate previous aliases for track %q: %w", source.Track.ID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM album_alias_tracks WHERE provider = ?", provider); err != nil {
		return fmt.Errorf("clear provider album aliases: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM track_sources WHERE provider = ?", provider); err != nil {
		return fmt.Errorf("clear provider sources: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tracks
		WHERE NOT EXISTS (SELECT 1 FROM track_sources WHERE track_sources.track_id = tracks.id)`); err != nil {
		return fmt.Errorf("clear orphaned tracks: %w", err)
	}
	trackStatement, err := tx.PrepareContext(ctx, `INSERT INTO tracks (
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
		cover_art_id = excluded.cover_art_id`)
	if err != nil {
		return fmt.Errorf("prepare track insert: %w", err)
	}
	defer trackStatement.Close()
	sourceStatement, err := tx.PrepareContext(ctx, `INSERT INTO track_sources (
		track_id, provider, source_key, discovered_at
	) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare source insert: %w", err)
	}
	defer sourceStatement.Close()
	aliasStatement, err := tx.PrepareContext(ctx, `INSERT INTO album_alias_tracks (
		provider, alias_id, track_id
	) VALUES (?, ?, ?)
	ON CONFLICT(provider, alias_id, track_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare album alias insert: %w", err)
	}
	defer aliasStatement.Close()

	for _, source := range sources {
		track := source.Track
		if source.Ref.Provider != provider {
			return fmt.Errorf("track %q belongs to provider %q, want %q", track.ID, source.Ref.Provider, provider)
		}
		albumArtistID := track.AlbumArtistID
		if albumArtistID == "" {
			albumArtistID = track.ArtistID
		}
		if _, err := trackStatement.ExecContext(ctx,
			track.ID, track.Title, track.Artist,
			track.ArtistID, track.Album, track.AlbumID, track.AlbumArtist, albumArtistID,
			track.TrackNumber, track.DiscNumber, track.Year, track.Genre, track.Duration.Milliseconds(),
			track.Size, track.BitRate, track.Suffix, track.ContentType, track.CoverArtID,
		); err != nil {
			return fmt.Errorf("insert track %q: %w", track.ID, err)
		}
		discoveredAt := ""
		if !source.DiscoveredAt.IsZero() {
			discoveredAt = source.DiscoveredAt.UTC().Format(time.RFC3339Nano)
		}
		if _, err := sourceStatement.ExecContext(ctx,
			track.ID, source.Ref.Provider, source.Ref.Key, discoveredAt,
		); err != nil {
			return fmt.Errorf("insert source for track %q: %w", track.ID, err)
		}
		if previousAlbumID := previousAlbums[track.ID]; previousAlbumID != "" && previousAlbumID != track.AlbumID {
			albumAliases = append(albumAliases, ports.AlbumAlias{AliasID: previousAlbumID, TrackID: track.ID})
		}
		for _, aliasID := range previousAliases[track.ID] {
			albumAliases = append(albumAliases, ports.AlbumAlias{AliasID: aliasID, TrackID: track.ID})
		}
	}
	for _, alias := range albumAliases {
		if alias.AliasID == "" || alias.TrackID == "" {
			continue
		}
		if _, ok := incomingTrackIDs[alias.TrackID]; !ok {
			return fmt.Errorf("album alias %q refers to unknown provider track %q", alias.AliasID, alias.TrackID)
		}
		if _, err := aliasStatement.ExecContext(ctx, provider, alias.AliasID, alias.TrackID); err != nil {
			return fmt.Errorf("insert album alias %q: %w", alias.AliasID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit catalog replacement: %w", err)
	}
	return nil
}

const trackColumns = `id, title, artist, artist_id, album, album_id, album_artist,
	album_artist_id, track_number, disc_number, year, genre, duration_ms,
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

func (c *Catalog) Sources(ctx context.Context, trackID string) ([]domain.SourceRef, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT provider, source_key FROM track_sources
		WHERE track_id = ? ORDER BY provider, source_key`, trackID)
	if err != nil {
		return nil, fmt.Errorf("query track sources: %w", err)
	}
	defer rows.Close()

	var sources []domain.SourceRef
	for rows.Next() {
		var source domain.SourceRef
		if err := rows.Scan(&source.Provider, &source.Key); err != nil {
			return nil, fmt.Errorf("scan track source: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate track sources: %w", err)
	}
	if len(sources) == 0 {
		return nil, ports.ErrNotFound
	}
	return sources, nil
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

func (c *Catalog) Genres(ctx context.Context) ([]domain.Genre, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT MIN(genre), COUNT(*), COUNT(DISTINCT album_id)
		FROM tracks WHERE genre <> '' GROUP BY genre COLLATE NOCASE
		ORDER BY MIN(genre) COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("query genres: %w", err)
	}
	defer rows.Close()

	var genres []domain.Genre
	for rows.Next() {
		var genre domain.Genre
		if err := rows.Scan(&genre.Name, &genre.SongCount, &genre.AlbumCount); err != nil {
			return nil, fmt.Errorf("scan genre: %w", err)
		}
		genres = append(genres, genre)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate genres: %w", err)
	}
	return genres, nil
}

func (c *Catalog) UpdateTrack(ctx context.Context, track domain.Track) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin track update: %w", err)
	}
	defer tx.Rollback()

	var previousAlbumID string
	if err := tx.QueryRowContext(ctx, "SELECT album_id FROM tracks WHERE id = ?", track.ID).
		Scan(&previousAlbumID); errors.Is(err, sql.ErrNoRows) {
		return ports.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("query track before update: %w", err)
	}
	albumArtistID := track.AlbumArtistID
	if albumArtistID == "" {
		albumArtistID = track.ArtistID
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tracks SET
		title = ?, artist = ?, artist_id = ?, album = ?, album_id = ?,
		album_artist = ?, album_artist_id = ?, track_number = ?, disc_number = ?,
		year = ?, genre = ?, duration_ms = ?, size_bytes = ?, bit_rate = ?, suffix = ?,
		content_type = ?, cover_art_id = ? WHERE id = ?`,
		track.Title, track.Artist, track.ArtistID, track.Album, track.AlbumID,
		track.AlbumArtist, albumArtistID, track.TrackNumber, track.DiscNumber,
		track.Year, track.Genre, track.Duration.Milliseconds(), track.Size, track.BitRate,
		track.Suffix, track.ContentType, track.CoverArtID, track.ID,
	); err != nil {
		return fmt.Errorf("update track %q: %w", track.ID, err)
	}
	if previousAlbumID != "" && previousAlbumID != track.AlbumID {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO album_alias_tracks (
			provider, alias_id, track_id
		) SELECT provider, ?, track_id FROM track_sources WHERE track_id = ?`,
			previousAlbumID, track.ID); err != nil {
			return fmt.Errorf("preserve previous album for track %q: %w", track.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit track update: %w", err)
	}
	return nil
}

func (c *Catalog) Albums(ctx context.Context, query ports.AlbumListQuery) ([]domain.Album, error) {
	order := "album COLLATE NOCASE, album_artist COLLATE NOCASE"
	switch query.Order {
	case "", ports.AlbumOrderName:
	case ports.AlbumOrderArtist:
		order = "album_artist COLLATE NOCASE, album COLLATE NOCASE"
	case ports.AlbumOrderNewest:
		order = "discovered_at DESC, album COLLATE NOCASE"
	case ports.AlbumOrderRandom:
		order = "RANDOM()"
	case ports.AlbumOrderYearAsc:
		order = "album_year ASC, album COLLATE NOCASE"
	case ports.AlbumOrderYearDesc:
		order = "album_year DESC, album COLLATE NOCASE"
	default:
		return nil, fmt.Errorf("unsupported album order %q", query.Order)
	}
	conditions := []string{}
	arguments := []any{}
	if query.FromYear != 0 || query.ToYear != 0 {
		lower, upper := min(query.FromYear, query.ToYear), max(query.FromYear, query.ToYear)
		conditions = append(conditions, "album_year BETWEEN ? AND ?")
		arguments = append(arguments, lower, upper)
	}
	if query.Genre != "" {
		conditions = append(conditions, `EXISTS (SELECT 1 FROM tracks genre_tracks
			WHERE genre_tracks.album_id = albums.album_id
			AND genre_tracks.genre = ? COLLATE NOCASE)`)
		arguments = append(arguments, query.Genre)
	}
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	arguments = append(arguments, query.Limit, query.Offset)
	rows, err := c.db.QueryContext(ctx, `WITH source_dates AS (
		SELECT track_id, MAX(discovered_at) AS discovered_at
		FROM track_sources GROUP BY track_id
	), albums AS (
		SELECT album_id, album, album_artist, album_artist_id,
			MIN(NULLIF(year, 0)) AS album_year, COUNT(*) AS song_count,
			SUM(duration_ms) AS duration_ms, MIN(NULLIF(cover_art_id, '')) AS cover_art_id,
			MIN(NULLIF(genre, '')) AS genre,
			MAX(source_dates.discovered_at) AS discovered_at
		FROM tracks LEFT JOIN source_dates ON source_dates.track_id = tracks.id
		GROUP BY album_id, album, album_artist, album_artist_id
	)
	SELECT album_id, album, album_artist, album_artist_id, album_year,
		song_count, duration_ms, cover_art_id, genre FROM albums `+where+`
	ORDER BY `+order+` LIMIT ? OFFSET ?`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("query all albums: %w", err)
	}
	defer rows.Close()
	return scanAlbums(rows)
}

func (c *Catalog) AlbumsByArtist(ctx context.Context, artistID string) ([]domain.Album, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT album_id, album, album_artist, album_artist_id,
		MIN(NULLIF(year, 0)), COUNT(*), SUM(duration_ms), MIN(NULLIF(cover_art_id, '')),
		MIN(NULLIF(genre, ''))
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
		var genre sql.NullString
		var durationMS int64
		if err := rows.Scan(&album.ID, &album.Name, &album.Artist, &album.ArtistID,
			&year, &album.SongCount, &durationMS, &coverArtID, &genre); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		album.Year = int(year.Int64)
		album.Duration = time.Duration(durationMS) * time.Millisecond
		album.CoverArtID = coverArtID.String
		album.Genre = genre.String
		albums = append(albums, album)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate albums: %w", err)
	}
	return albums, nil
}

func (c *Catalog) TracksByAlbum(ctx context.Context, albumID string) ([]domain.Track, error) {
	tracks, err := c.tracksByCanonicalAlbum(ctx, albumID)
	if err != nil || len(tracks) > 0 {
		return tracks, err
	}
	rows, err := c.db.QueryContext(ctx, "SELECT "+trackColumns+` FROM tracks
		WHERE EXISTS (
			SELECT 1 FROM album_alias_tracks
			WHERE album_alias_tracks.alias_id = ?
				AND album_alias_tracks.track_id = tracks.id
		) ORDER BY disc_number, track_number, title COLLATE NOCASE`, albumID)
	if err != nil {
		return nil, fmt.Errorf("query album alias tracks: %w", err)
	}
	defer rows.Close()
	return scanTracks(rows, "album alias")
}

func (c *Catalog) tracksByCanonicalAlbum(ctx context.Context, albumID string) ([]domain.Track, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT "+trackColumns+` FROM tracks WHERE album_id = ?
		ORDER BY disc_number, track_number, title COLLATE NOCASE`, albumID)
	if err != nil {
		return nil, fmt.Errorf("query album tracks: %w", err)
	}
	defer rows.Close()

	return scanTracks(rows, "album")
}

func scanTracks(rows *sql.Rows, kind string) ([]domain.Track, error) {
	var tracks []domain.Track
	for rows.Next() {
		track, err := scanTrack(rows)
		if err != nil {
			return nil, fmt.Errorf("scan %s track: %w", kind, err)
		}
		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s tracks: %w", kind, err)
	}
	return tracks, nil
}

func (c *Catalog) Search(ctx context.Context, query ports.CatalogSearch) (ports.CatalogSearchResult, error) {
	artists, err := c.Artists(ctx)
	if err != nil {
		return ports.CatalogSearchResult{}, err
	}
	allResults := int(^uint(0) >> 1)
	albums, err := c.Albums(ctx, ports.AlbumListQuery{Limit: allResults})
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
		&track.ID, &track.Title, &track.Artist,
		&track.ArtistID, &track.Album, &track.AlbumID, &track.AlbumArtist,
		&track.AlbumArtistID,
		&track.TrackNumber, &track.DiscNumber, &track.Year, &track.Genre, &durationMS,
		&track.Size, &track.BitRate, &track.Suffix, &track.ContentType, &track.CoverArtID,
	)
	track.Duration = time.Duration(durationMS) * time.Millisecond
	return track, err
}
