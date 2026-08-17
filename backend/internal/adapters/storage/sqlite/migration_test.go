package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/randomtoy/peerphonic/backend/migrations"
)

func TestMigrationSeparatesLegacyTrackSources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"001_create_catalog.sql",
		"002_add_cover_art.sql",
		"003_add_album_artist_identity.sql",
	} {
		contents, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("apply legacy migration %s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO schema_migrations(version, applied_at) VALUES (?, '2026-01-01T00:00:00Z')", name,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tracks (
		id, provider, source_key, title, artist, artist_id, album, album_id,
		album_artist, album_artist_id, suffix, content_type
	) VALUES (
		'track-1', 'local', 'Artist/Album/Song.mp3', 'Song', 'Artist', 'artist-1',
		'Album', 'album-1', 'Artist', 'artist-1', 'mp3', 'audio/mpeg'
	)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	catalog, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() migrated database error = %v", err)
	}
	defer catalog.Close()
	track, err := catalog.Track(ctx, "track-1")
	if err != nil || track.Title != "Song" {
		t.Fatalf("Track() = %#v, %v", track, err)
	}
	sources, err := catalog.Sources(ctx, "track-1")
	if err != nil || len(sources) != 1 || sources[0].Provider != "local" ||
		sources[0].Key != "Artist/Album/Song.mp3" {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
}
