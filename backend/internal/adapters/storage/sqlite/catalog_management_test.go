package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func TestArtistAliasesAreReversibleCatalogViews(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	various := domain.CanonicalArtistID("Various")
	variousArtists := domain.CanonicalArtistID("Various Artists")
	for _, source := range []domain.TrackSource{
		{Track: domain.Track{ID: "one", Title: "One", Artist: "Various", ArtistID: various,
			Album: "First", AlbumID: domain.CanonicalAlbumID("Various", "First"),
			AlbumArtist: "Various", AlbumArtistID: various, Suffix: "mp3", ContentType: "audio/mpeg"},
			Ref: domain.SourceRef{Provider: "local", Key: "one"}},
		{Track: domain.Track{ID: "two", Title: "Two", Artist: "Various Artists", ArtistID: variousArtists,
			Album: "Second", AlbumID: domain.CanonicalAlbumID("Various Artists", "Second"),
			AlbumArtist: "Various Artists", AlbumArtistID: variousArtists, Suffix: "mp3", ContentType: "audio/mpeg"},
			Ref: domain.SourceRef{Provider: "local", Key: "two"}},
	} {
		if err := catalog.SaveTrackSource(ctx, source); err != nil {
			t.Fatal(err)
		}
	}
	alias, err := catalog.SetArtistAlias(ctx, various, variousArtists)
	if err != nil || alias.TargetID != variousArtists {
		t.Fatalf("SetArtistAlias() = %#v, %v", alias, err)
	}
	artists, err := catalog.Artists(ctx)
	if err != nil || len(artists) != 1 || artists[0].AlbumCount != 2 {
		t.Fatalf("Artists() = %#v, %v", artists, err)
	}
	albums, err := catalog.AlbumsByArtist(ctx, various)
	if err != nil || len(albums) != 2 {
		t.Fatalf("AlbumsByArtist(alias) = %#v, %v", albums, err)
	}
	if err := catalog.DeleteArtistAlias(ctx, various); err != nil {
		t.Fatal(err)
	}
	artists, err = catalog.Artists(ctx)
	if err != nil || len(artists) != 2 {
		t.Fatalf("Artists() after delete = %#v, %v", artists, err)
	}
}

func TestUpdateTrackMetadataKeepsStableTrackID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	track := domain.Track{ID: "stable", Title: "Old", Artist: "Old Artist", ArtistID: "old",
		Album: "Old Album", AlbumID: "old-album", AlbumArtist: "Old Artist", AlbumArtistID: "old",
		Suffix: "mp3", ContentType: "audio/mpeg"}
	if err := catalog.SaveTrackSource(ctx, domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
	}); err != nil {
		t.Fatal(err)
	}
	title, artist, album := "New", "New Artist", "New Album"
	updated, err := catalog.UpdateTrackMetadata(ctx, track.ID, domain.TrackMetadataPatch{
		Title: &title, Artist: &artist, Album: &album,
	})
	if err != nil || updated.ID != track.ID || updated.Title != title ||
		updated.ArtistID != domain.CanonicalArtistID(artist) ||
		updated.AlbumID != domain.CanonicalAlbumID("Old Artist", album) {
		t.Fatalf("UpdateTrackMetadata() = %#v, %v", updated, err)
	}
}
