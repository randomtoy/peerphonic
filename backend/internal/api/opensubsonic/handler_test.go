package opensubsonic

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/providers/local"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
)

func newTestHandler(t *testing.T) (http.Handler, domain.Track) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Artist", "Album", "song.mp3")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { catalog.Close() })
	track := domain.Track{
		ID: "track_test", Title: "Song", Artist: "Artist", ArtistID: "artist_test",
		Album: "Album", AlbumID: "album_test", AlbumArtist: "Artist",
		Source:      domain.SourceRef{Provider: local.Name, Key: "Artist/Album/song.mp3"},
		TrackNumber: 1, Size: 10, Suffix: "mp3", ContentType: "audio/mpeg",
	}
	if err := catalog.ReplaceProviderTracks(ctx, local.Name, []domain.Track{track}); err != nil {
		t.Fatal(err)
	}
	provider, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	streams := services.NewStreamingService(catalog, provider)
	return NewHandler(catalog, streams, "alice", "secret"), track
}

func TestPingSupportsTokenAuthenticationAndJSON(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandler(t)
	salt := "salt"
	token := fmt.Sprintf("%x", md5.Sum([]byte("secret"+salt)))
	request := httptest.NewRequest(http.MethodGet, "/rest/ping.view?u=alice&s="+salt+"&t="+token+"&f=json", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"ok"`) || !strings.Contains(response.Body.String(), `"openSubsonic":true`) {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestBrowseAndStreamRange(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	auth := "?u=alice&p=secret&f=json"
	browse := httptest.NewRecorder()
	handler.ServeHTTP(browse, httptest.NewRequest(http.MethodGet, "/rest/getMusicDirectory"+auth+"&id=album_test", nil))
	if browse.Code != http.StatusOK || !strings.Contains(browse.Body.String(), `"title":"Song"`) {
		t.Fatalf("browse status = %d, body = %s", browse.Code, browse.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/rest/stream?u=alice&p=secret&id="+track.ID, nil)
	request.Header.Set("Range", "bytes=2-5")
	stream := httptest.NewRecorder()
	handler.ServeHTTP(stream, request)
	if stream.Code != http.StatusPartialContent {
		t.Fatalf("stream status = %d, body = %s", stream.Code, stream.Body.String())
	}
	if stream.Body.String() != "2345" {
		t.Fatalf("stream body = %q, want 2345", stream.Body.String())
	}
}

func TestRejectsWrongPassword(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rest/ping?u=alice&p=wrong", nil))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `code="40"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
