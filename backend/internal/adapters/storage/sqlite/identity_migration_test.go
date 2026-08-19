package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func TestCanonicalIdentityMigrationMergesProviderArtistsAndAlbums(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.db")
	catalog, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	if err := catalog.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"DELETE FROM schema_migrations WHERE version = ?", canonicalCatalogIdentityMigration,
	); err != nil {
		t.Fatal(err)
	}
	for _, values := range [][]any{
		{"torrent-track", "Ron Pope", "torrent-artist", "Atlanta", "torrent-album"},
		{"soulseek-track", "ron pope", "soulseek-artist", " atlanta ", "soulseek-album"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO tracks (
			id, title, artist, artist_id, album, album_id, album_artist,
			album_artist_id, suffix, content_type
		) VALUES (?, 'Song', ?, ?, ?, ?, ?, ?, 'mp3', 'audio/mpeg')`,
			values[0], values[1], values[2], values[3], values[4], values[1], values[2],
		); err != nil {
			t.Fatal(err)
		}
	}
	for _, values := range [][]any{
		{"artist", "torrent-artist", "2026-01-01T00:00:00Z", 2, 3, "2026-01-01T00:00:00Z"},
		{"artist", "soulseek-artist", "2026-02-01T00:00:00Z", 5, 4, "2026-02-01T00:00:00Z"},
		{"album", "torrent-album", "", 1, 2, ""},
		{"album", "soulseek-album", "", 4, 6, ""},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO media_annotations (
			owner, media_type, media_id, starred_at, rating, play_count, last_played_at
		) VALUES ('admin', ?, ?, ?, ?, ?, ?)`, values...); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	catalog, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("migrate catalog: %v", err)
	}
	defer catalog.Close()

	artistID := domain.CanonicalArtistID("Ron Pope")
	albumID := domain.CanonicalAlbumID("Ron Pope", "Atlanta")
	artists, err := catalog.Artists(ctx)
	if err != nil || len(artists) != 1 || artists[0].ID != artistID {
		t.Fatalf("Artists() = %#v, %v", artists, err)
	}
	albums, err := catalog.AlbumsByArtist(ctx, artistID)
	if err != nil || len(albums) != 1 || albums[0].ID != albumID || albums[0].SongCount != 2 {
		t.Fatalf("AlbumsByArtist() = %#v, %v", albums, err)
	}
	tracks, err := catalog.TracksByAlbum(ctx, albumID)
	if err != nil || len(tracks) != 2 {
		t.Fatalf("TracksByAlbum() = %#v, %v", tracks, err)
	}
	for _, track := range tracks {
		if track.ArtistID != artistID || track.AlbumArtistID != artistID || track.AlbumID != albumID {
			t.Errorf("track identity was not normalized: %#v", track)
		}
	}

	annotations, err := catalog.MediaAnnotations(ctx, "admin")
	if err != nil || len(annotations) != 2 {
		t.Fatalf("MediaAnnotations() = %#v, %v", annotations, err)
	}
	for _, annotation := range annotations {
		switch annotation.Media.Type {
		case domain.MediaArtist:
			if annotation.Media.ID != artistID || annotation.Rating != 5 || annotation.PlayCount != 7 {
				t.Errorf("merged artist annotation = %#v", annotation)
			}
		case domain.MediaAlbum:
			if annotation.Media.ID != albumID || annotation.Rating != 4 || annotation.PlayCount != 8 {
				t.Errorf("merged album annotation = %#v", annotation)
			}
		default:
			t.Errorf("unexpected annotation = %#v", annotation)
		}
	}
}
