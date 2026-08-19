package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func TestLogicalTrackMigrationPreservesReferencesAndAggregatesSources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.db")
	catalog, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = ?", logicalTrackMigration); err != nil {
		t.Fatal(err)
	}
	tracks := []struct {
		id, title, provider, key, suffix string
	}{
		{"local-old", "Prison Song", "local", "SOAD/Toxicity/01.flac", "flac"},
		{"torrent-old", "01 - PRISON SONG", "torrent", "hash/SOAD/Toxicity/01.mp3", "mp3"},
	}
	for _, item := range tracks {
		if _, err := db.ExecContext(ctx, `INSERT INTO tracks (
			id,title,artist,artist_id,album,album_id,album_artist,album_artist_id,
			track_number,suffix,content_type
		) VALUES (?,?,'System of a Down',?,'Toxicity',?,'System of a Down',?,1,?,'audio/test')`,
			item.id, item.title, domain.CanonicalArtistID("System of a Down"),
			domain.CanonicalAlbumID("System of a Down", "Toxicity"),
			domain.CanonicalArtistID("System of a Down"), item.suffix); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO track_sources(track_id,provider,source_key) VALUES (?,?,?)",
			item.id, item.provider, item.key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO playlists(
		id,owner,name,created_at,changed_at) VALUES ('list','admin','Test',?,?)`,
		time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO playlist_tracks(playlist_id,position,track_id) VALUES ('list',0,'torrent-old')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO media_annotations(
		owner,media_type,media_id,rating,play_count) VALUES
		('admin','song','local-old',2,3),('admin','song','torrent-old',5,4)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	catalog, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	logicalID := domain.CanonicalTrackID(domain.Track{
		Title: "Prison Song", Artist: "System of a Down", Album: "Toxicity",
		AlbumArtist: "System of a Down", TrackNumber: 1,
	})
	track, err := catalog.Track(ctx, "torrent-old")
	if err != nil || track.ID != logicalID || track.Suffix != "flac" {
		t.Fatalf("Track(alias) = %#v, %v", track, err)
	}
	sources, err := catalog.Sources(ctx, logicalID)
	if err != nil || len(sources) != 2 || sources[0].Provider != "local" {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
	playlist, err := catalog.Playlist(ctx, "list")
	if err != nil || len(playlist.Tracks) != 1 || playlist.Tracks[0].ID != logicalID {
		t.Fatalf("Playlist() = %#v, %v", playlist, err)
	}
	annotations, err := catalog.MediaAnnotations(ctx, "admin")
	if err != nil || len(annotations) != 1 || annotations[0].Media.ID != logicalID ||
		annotations[0].Rating != 5 || annotations[0].PlayCount != 7 {
		t.Fatalf("MediaAnnotations() = %#v, %v", annotations, err)
	}
}
