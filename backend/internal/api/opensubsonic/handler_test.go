package opensubsonic

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blobfs "github.com/randomtoy/peerphonic/backend/internal/adapters/blob/filesystem"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/providers/local"
	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

func newTestHandler(t *testing.T, scans ...scanController) (http.Handler, domain.Track) {
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
	blobs, err := blobfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	artwork := services.NewArtworkService(blobs)
	coverArtID, err := artwork.Put(ctx, testCoverPNG)
	if err != nil {
		t.Fatal(err)
	}
	track := domain.Track{
		ID: "track_test", Title: "Song", Artist: "Artist", ArtistID: "artist_test",
		Album: "Album", AlbumID: "album_test", AlbumArtist: "Artist",
		TrackNumber: 1, Year: 2026, Genre: "Rock", Size: 10, Suffix: "mp3", ContentType: "audio/mpeg", CoverArtID: coverArtID,
	}
	source := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: local.Name, Key: "Artist/Album/song.mp3"},
	}
	aliases := []ports.AlbumAlias{{AliasID: "album_legacy", TrackID: track.ID}}
	if err := catalog.ReplaceProviderTracks(ctx, local.Name, []domain.TrackSource{source}, aliases); err != nil {
		t.Fatal(err)
	}
	provider, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	streams := services.NewStreamingService(catalog, provider)
	return NewHandler(catalog, streams, artwork, "alice", "secret", scans...), track
}

var testCoverPNG = func() []byte {
	var data bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&data, picture); err != nil {
		panic(err)
	}
	return data.Bytes()
}()

type scanControllerStub struct {
	started bool
	status  scanner.Status
}

func (s *scanControllerStub) Start() bool {
	s.started = true
	s.status.Scanning = true
	return true
}

func (s *scanControllerStub) Status() scanner.Status { return s.status }

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

func TestMetadataOnlyTorrentTrackIsTemporarilyUnavailable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	track := domain.Track{
		ID: "torrent-track", Title: "Remote", Artist: "Artist", ArtistID: "artist",
		Album: "Album", AlbumID: "album", AlbumArtist: "Artist",
	}
	source := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{
			Provider: torrentprovider.Name, Key: strings.Repeat("0", 40) + "/Remote.mp3",
		},
	}
	if err := catalog.ReplaceProviderTracks(ctx, torrentprovider.Name, []domain.TrackSource{source}, nil); err != nil {
		t.Fatal(err)
	}
	streams := services.NewStreamingService(catalog, torrentprovider.New())
	handler := NewHandler(catalog, streams, nil, "alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/rest/stream?u=alice&p=secret&id="+track.ID, nil))
	if response.Code != http.StatusServiceUnavailable ||
		!strings.Contains(response.Body.String(), "Song source is not available yet") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGetCoverArtReturnsStoredImage(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	request := httptest.NewRequest(http.MethodGet,
		"/rest/getCoverArt?u=alice&p=secret&id="+track.CoverArtID+"&size=64", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("status = %d, content type = %q, body = %s",
			response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	if response.Body.String() != string(testCoverPNG) {
		t.Fatalf("body = %q", response.Body.String())
	}
}

func TestGetCoverArtResizesToRequestedMaximum(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	request := httptest.NewRequest(http.MethodGet,
		"/rest/getCoverArt?u=alice&p=secret&id="+track.CoverArtID+"&size=1", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 1 || config.Height != 1 {
		t.Fatalf("cover dimensions = %dx%d", config.Width, config.Height)
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

func TestID3BrowsingEndpointsUsedByAmperfy(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	auth := "?u=alice&p=secret&v=1.16.1&c=Amperfy"
	tests := []struct {
		path     string
		contains []string
	}{
		{path: "/rest/getGenres.view" + auth, contains: []string{
			`<genre songCount="1" albumCount="1">Rock</genre>`,
		}},
		{path: "/rest/getArtists.view" + auth, contains: []string{
			"<artists", `id="artist_test"`, `albumCount="1"`,
		}},
		{path: "/rest/getArtist.view" + auth + "&id=artist_test", contains: []string{
			`<artist id="artist_test"`, `<album id="album_test"`,
		}},
		{path: "/rest/getAlbumList2.view" + auth + "&type=alphabeticalByName&size=500&offset=0", contains: []string{
			"<albumList2>", `<album id="album_test"`,
		}},
		{path: "/rest/getAlbum.view" + auth + "&id=album_test", contains: []string{
			`<album id="album_test"`, `<song id="` + track.ID + `"`, `albumId="album_test"`,
			`coverArt="` + track.CoverArtID + `"`,
		}},
		{path: "/rest/getAlbum.view" + auth + "&id=album_legacy", contains: []string{
			`<album id="album_legacy"`, `<song id="` + track.ID + `"`, `albumId="album_legacy"`,
		}},
		{path: "/rest/getSongsByGenre.view" + auth + "&genre=Rock&count=1&offset=0", contains: []string{
			"<songsByGenre>", `<song id="` + track.ID + `"`, `genre="Rock"`,
		}},
		{path: "/rest/getSong.view" + auth + "&id=" + track.ID, contains: []string{
			`<song id="` + track.ID + `"`,
		}},
		{path: "/rest/search3.view" + auth + "&query=artist&artistCount=20&albumCount=20&songCount=20", contains: []string{
			"<searchResult3>", `<artist id="artist_test"`, `<album id="album_test"`, `<song id="` + track.ID + `"`,
		}},
		{path: "/rest/getPlaylists.view" + auth, contains: []string{"<playlists></playlists>"}},
		{path: "/rest/getOpenSubsonicExtensions.view" + auth, contains: []string{
			"<openSubsonicExtensions></openSubsonicExtensions>",
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			for _, expected := range test.contains {
				if !strings.Contains(response.Body.String(), expected) {
					t.Errorf("body does not contain %q: %s", expected, response.Body.String())
				}
			}
		})
	}
}

func TestSongsByGenreValidatesAndPages(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	auth := "?u=alice&p=secret&f=json"
	tests := []struct {
		name       string
		parameters string
		status     int
		contains   string
	}{
		{name: "missing genre", status: http.StatusBadRequest, contains: "genre"},
		{name: "invalid count", parameters: "&genre=Rock&count=-1", status: http.StatusBadRequest, contains: "count"},
		{name: "invalid offset", parameters: "&genre=Rock&offset=no", status: http.StatusBadRequest, contains: "offset"},
		{name: "song", parameters: "&genre=rock&count=1", status: http.StatusOK, contains: `"id":"` + track.ID + `"`},
		{name: "empty page", parameters: "&genre=Rock&offset=1", status: http.StatusOK, contains: `"song":[]`},
		{name: "unknown folder", parameters: "&genre=Rock&musicFolderId=other", status: http.StatusOK, contains: `"song":[]`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
				"/rest/getSongsByGenre"+auth+test.parameters, nil))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAlbumList2ValidatesAndSupportsListTypes(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandler(t)
	auth := "?u=alice&p=secret&f=json"
	tests := []struct {
		name       string
		parameters string
		status     int
		contains   string
	}{
		{name: "missing type", status: http.StatusBadRequest, contains: "required parameter type"},
		{name: "unknown type", parameters: "&type=unknown", status: http.StatusBadRequest, contains: "unsupported album list type"},
		{name: "missing year range", parameters: "&type=byYear", status: http.StatusBadRequest, contains: "fromYear"},
		{name: "missing genre", parameters: "&type=byGenre", status: http.StatusBadRequest, contains: "genre"},
		{name: "genre", parameters: "&type=byGenre&genre=rock", status: http.StatusOK, contains: `"album_test"`},
		{name: "activity list without history", parameters: "&type=recent", status: http.StatusOK, contains: `"album":[]`},
		{name: "year range", parameters: "&type=byYear&fromYear=2026&toYear=2020", status: http.StatusOK, contains: `"album_test"`},
		{name: "alphabetical artist", parameters: "&type=alphabeticalByArtist", status: http.StatusOK, contains: `"album_test"`},
		{name: "newest", parameters: "&type=newest", status: http.StatusOK, contains: `"album_test"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
				"/rest/getAlbumList2"+auth+test.parameters, nil))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSearch3SupportsEmptyQueryAndIndependentCounts(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	request := httptest.NewRequest(http.MethodGet,
		"/rest/search3?u=alice&p=secret&f=json&query=&artistCount=0&albumCount=0&songCount=1", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, expected := range []string{`"artist":[]`, `"album":[]`, `"song":[`, `"id":"` + track.ID + `"`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("body does not contain %q: %s", expected, response.Body.String())
		}
	}
}

func TestSearch3ValidatesParameters(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandler(t)
	for _, path := range []string{
		"/rest/search3?u=alice&p=secret&f=json",
		"/rest/search3?u=alice&p=secret&f=json&query=song&songCount=-1",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":10`) {
			t.Fatalf("path = %s, status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
}

func TestScanEndpoints(t *testing.T) {
	t.Parallel()

	controller := &scanControllerStub{status: scanner.Status{Count: 507}}
	handler, _ := newTestHandler(t, controller)
	request := httptest.NewRequest(http.MethodGet,
		"/rest/startScan.view?u=alice&p=secret&v=1.16.1&c=test", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !controller.started {
		t.Fatalf("status = %d, started = %v, body = %s", response.Code, controller.started, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `scanning="true"`) ||
		!strings.Contains(response.Body.String(), `count="507"`) {
		t.Fatalf("body = %s", response.Body.String())
	}
}
