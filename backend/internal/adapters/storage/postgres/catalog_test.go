package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestCatalogIntegration(t *testing.T) {
	connectionURL := os.Getenv("PEERPHONIC_TEST_POSTGRES_URL")
	if connectionURL == "" {
		t.Skip("PEERPHONIC_TEST_POSTGRES_URL is not configured")
	}
	ctx := context.Background()
	catalog, err := Open(ctx, connectionURL)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer catalog.Close()

	track := domain.Track{
		ID: "track-postgres", Title: "Track", Artist: "Artist", ArtistID: "artist-postgres",
		Album: "Album", AlbumID: "album-postgres", AlbumArtist: "Artist",
		AlbumArtistID: "artist-postgres", Genre: "Post Rock", Duration: 3 * time.Minute,
		Size: 1234, Suffix: "flac", ContentType: "audio/flac",
	}
	source := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "local", Key: "Artist/Album/Track.flac"},
		DiscoveredAt: time.Now().UTC(),
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
		t.Fatalf("ReplaceProviderTracks() error = %v", err)
	}
	stored, err := catalog.Track(ctx, track.ID)
	if err != nil || stored.Title != track.Title {
		t.Fatalf("Track() = %#v, %v", stored, err)
	}
	genres, err := catalog.Genres(ctx)
	if err != nil || len(genres) != 1 || genres[0].Name != track.Genre {
		t.Fatalf("Genres() = %#v, %v", genres, err)
	}
	if err := catalog.SaveCacheEntry(ctx, domain.CacheEntry{
		Key: "postgres-cache", Size: 42, LastAccessed: time.Now().UTC(), Pinned: true,
	}); err != nil {
		t.Fatalf("SaveCacheEntry() error = %v", err)
	}
	entry, err := catalog.CacheEntry(ctx, "postgres-cache")
	if err != nil || !entry.Pinned {
		t.Fatalf("CacheEntry() = %#v, %v", entry, err)
	}
	credential := ports.UserCredential{User: domain.User{
		Username: "postgres-user", Role: domain.UserRoleUser, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}, PasswordHash: []byte("hash"), EncryptedToken: []byte("token")}
	if err := catalog.CreateUser(ctx, credential); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := catalog.UserCredential(ctx, credential.User.Username); err != nil {
		t.Fatalf("UserCredential() error = %v", err)
	}
	if err := catalog.SavePlaylist(ctx, domain.Playlist{
		ID: "playlist-postgres", Owner: credential.User.Username, Name: "Playlist", Public: true,
		Created: time.Now().UTC(), Changed: time.Now().UTC(), Tracks: []domain.Track{track},
	}); err != nil {
		t.Fatalf("SavePlaylist() error = %v", err)
	}
	playlists, err := catalog.Playlists(ctx, "another-user")
	if err != nil || len(playlists) != 1 {
		t.Fatalf("Playlists() = %#v, %v", playlists, err)
	}
	if err := catalog.SetTrackPinned(ctx, track.ID, true); err != nil {
		t.Fatalf("SetTrackPinned() error = %v", err)
	}
	if pinned, err := catalog.TrackPinned(ctx, track.ID); err != nil || !pinned {
		t.Fatalf("TrackPinned() = %v, %v", pinned, err)
	}
}
