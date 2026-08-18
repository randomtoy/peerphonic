package opensubsonic

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
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
	"time"

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

type permissionAuthenticator struct{ user domain.User }

func (a permissionAuthenticator) AuthenticatePassword(
	context.Context, string, string,
) (domain.User, error) {
	return a.user, nil
}

func (a permissionAuthenticator) AuthenticateToken(
	context.Context, string, string, string,
) (domain.User, error) {
	return a.user, nil
}

type discoveryProviderStub struct {
	results []domain.TrackSource
	media   []byte
	resolve func(domain.SourceRef) (ports.ResolvedSource, error)
}

func (p *discoveryProviderStub) Name() string { return "soulseek" }

func (p *discoveryProviderStub) Search(
	context.Context, domain.SearchQuery,
) ([]domain.TrackSource, error) {
	return p.results, nil
}

func (p *discoveryProviderStub) Resolve(
	_ context.Context, _ string, ref domain.SourceRef,
) (ports.ResolvedSource, error) {
	if p.resolve != nil {
		return p.resolve(ref)
	}
	return ports.ResolvedSource{
		Content: &memoryReadSeekCloser{Reader: bytes.NewReader(p.media)},
		Name:    "remote.flac", ContentType: "audio/flac", Size: int64(len(p.media)),
	}, nil
}

func TestStreamRefreshesUnavailableDiscoveredSources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { catalog.Close() })
	track := domain.Track{
		ID: "soulseek-track", Artist: "System of a Down", ArtistID: "artist-system",
		Album: "Toxicity", AlbumID: "album-toxicity", AlbumArtist: "System of a Down",
		Title: "01 - Prison Song", Suffix: "mp3", ContentType: "audio/mpeg", Size: 10,
	}
	stale := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "soulseek", Key: "stale-peer"},
		DiscoveredAt: time.Now().Add(-time.Hour),
	}
	if err := catalog.SaveTrackSource(ctx, stale); err != nil {
		t.Fatal(err)
	}
	fresh := domain.TrackSource{
		Track:        domain.Track{Artist: "System Of A Down", Title: "Prison Song", Suffix: "mp3"},
		Ref:          domain.SourceRef{Provider: "soulseek", Key: "fresh-peer"},
		DisplayPath:  "System Of A Down/Toxicity/01 Prison Song.mp3",
		Availability: domain.SourceAvailability{FreeUploadSlot: true},
	}
	provider := &discoveryProviderStub{results: []domain.TrackSource{fresh}}
	provider.resolve = func(ref domain.SourceRef) (ports.ResolvedSource, error) {
		if ref.Key == "stale-peer" {
			return ports.ResolvedSource{}, fmt.Errorf("%w: File not shared", ports.ErrSourceUnavailable)
		}
		return ports.ResolvedSource{
			Content: &memoryReadSeekCloser{Reader: bytes.NewReader([]byte("fresh-media"))},
			Name:    "fresh.mp3", ContentType: "audio/mpeg", Size: 11,
		}, nil
	}
	discovery := services.NewDiscoveryService(provider, catalog)
	streams := services.NewStreamingService(catalog, provider)
	handler := NewHandlerWithAuthenticatorAndDiscovery(
		catalog, streams, nil, permissionAuthenticator{user: domain.User{
			Role: domain.UserRoleUser, Permissions: []domain.Permission{domain.PermissionSoulseekClient},
		}}, discovery,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/rest/stream?u=listener&p=secret&id="+track.ID, nil))
	if response.Code != http.StatusOK || response.Body.String() != "fresh-media" {
		t.Fatalf("stream status = %d, body = %q", response.Code, response.Body.String())
	}
	sources, err := catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 2 || sources[0] != fresh.Ref {
		t.Fatalf("refreshed sources = %#v, %v", sources, err)
	}
}

type memoryReadSeekCloser struct{ *bytes.Reader }

func (*memoryReadSeekCloser) Close() error { return nil }

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

func TestMediaAnnotationLifecycle(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	auth := "?u=alice&p=secret&f=json"
	request := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}

	star := request("/rest/star.view" + auth + "&id=" + track.ID +
		"&albumId=" + track.AlbumID + "&artistId=" + track.ArtistID)
	if star.Code != http.StatusOK || !strings.Contains(star.Body.String(), `"status":"ok"`) {
		t.Fatalf("star status = %d, body = %s", star.Code, star.Body.String())
	}
	rating := request("/rest/setRating.view" + auth + "&id=" + track.ID + "&rating=4")
	if rating.Code != http.StatusOK {
		t.Fatalf("setRating status = %d, body = %s", rating.Code, rating.Body.String())
	}
	scrobble := request("/rest/scrobble.view" + auth + "&id=" + track.ID +
		"&id=" + track.ID + "&time=1786982400000&time=1786982460000")
	if scrobble.Code != http.StatusOK {
		t.Fatalf("scrobble status = %d, body = %s", scrobble.Code, scrobble.Body.String())
	}

	songResponse := request("/rest/getSong.view" + auth + "&id=" + track.ID)
	var songEnvelope struct {
		Response struct {
			Song child `json:"song"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(songResponse.Body.Bytes(), &songEnvelope); err != nil {
		t.Fatal(err)
	}
	song := songEnvelope.Response.Song
	if song.Starred == "" || song.UserRating != 4 || song.PlayCount != 2 || song.Played == "" {
		t.Fatalf("annotated song = %#v", song)
	}

	starredResponse := request("/rest/getStarred2.view" + auth)
	var starredEnvelope struct {
		Response struct {
			Starred starredLibrary `json:"starred2"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(starredResponse.Body.Bytes(), &starredEnvelope); err != nil {
		t.Fatal(err)
	}
	starred := starredEnvelope.Response.Starred
	if len(starred.Songs) != 1 || len(starred.Albums) != 1 || len(starred.Artists) != 1 ||
		starred.Songs[0].UserRating != 4 || starred.Songs[0].PlayCount != 2 {
		t.Fatalf("getStarred2 = %#v", starred)
	}
	starredAlbums := request("/rest/getAlbumList2.view" + auth + "&type=starred&size=10")
	if starredAlbums.Code != http.StatusOK || !strings.Contains(starredAlbums.Body.String(), `"id":"album_test"`) {
		t.Fatalf("starred album list status = %d, body = %s", starredAlbums.Code, starredAlbums.Body.String())
	}

	unstar := request("/rest/unstar.view" + auth + "&id=" + track.ID)
	if unstar.Code != http.StatusOK {
		t.Fatalf("unstar status = %d, body = %s", unstar.Code, unstar.Body.String())
	}
	songResponse = request("/rest/getSong.view" + auth + "&id=" + track.ID)
	songEnvelope = struct {
		Response struct {
			Song child `json:"song"`
		} `json:"subsonic-response"`
	}{}
	if err := json.Unmarshal(songResponse.Body.Bytes(), &songEnvelope); err != nil {
		t.Fatal(err)
	}
	song = songEnvelope.Response.Song
	if song.Starred != "" || song.UserRating != 4 || song.PlayCount != 2 {
		t.Fatalf("song after unstar = %#v", song)
	}
}

func TestMediaAnnotationValidation(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	tests := []string{
		"/rest/star.view?u=alice&p=secret&f=json",
		"/rest/setRating.view?u=alice&p=secret&f=json&id=" + track.ID + "&rating=6",
		"/rest/scrobble.view?u=alice&p=secret&f=json&id=" + track.ID + "&time=invalid",
	}
	for _, path := range tests {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":10`) {
			t.Fatalf("request %s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
}

func TestPlayQueueLifecycle(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	auth := "?u=alice&p=secret&f=json&c=test-client"
	request := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}

	empty := request("/rest/getPlayQueue.view" + auth)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"username":"alice"`) ||
		!strings.Contains(empty.Body.String(), `"entry":[]`) {
		t.Fatalf("empty getPlayQueue status = %d, body = %s", empty.Code, empty.Body.String())
	}
	saved := request("/rest/savePlayQueue.view" + auth + "&id=" + track.ID +
		"&id=" + track.ID + "&current=" + track.ID + "&position=42000")
	if saved.Code != http.StatusOK {
		t.Fatalf("savePlayQueue status = %d, body = %s", saved.Code, saved.Body.String())
	}

	loaded := request("/rest/getPlayQueue.view" + auth)
	var envelope struct {
		Response struct {
			Queue playQueue `json:"playQueue"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(loaded.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	queue := envelope.Response.Queue
	if queue.Current != track.ID || queue.Position != 42000 || queue.Username != "alice" ||
		queue.ChangedBy != "test-client" || queue.Changed == "" || len(queue.Entries) != 2 {
		t.Fatalf("getPlayQueue = %#v", queue)
	}

	cleared := request("/rest/savePlayQueue.view" + auth)
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear savePlayQueue status = %d, body = %s", cleared.Code, cleared.Body.String())
	}
	empty = request("/rest/getPlayQueue.view" + auth)
	if !strings.Contains(empty.Body.String(), `"entry":[]`) ||
		strings.Contains(empty.Body.String(), `"current"`) {
		t.Fatalf("cleared getPlayQueue body = %s", empty.Body.String())
	}
}

func TestPlayQueueValidation(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	tests := []string{
		"/rest/savePlayQueue.view?u=alice&p=secret&f=json&id=" + track.ID,
		"/rest/savePlayQueue.view?u=alice&p=secret&f=json&id=" + track.ID + "&current=missing",
		"/rest/savePlayQueue.view?u=alice&p=secret&f=json&position=-1",
		"/rest/savePlayQueue.view?u=alice&p=secret&f=json&id=missing&current=missing",
	}
	for _, path := range tests {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest && response.Code != http.StatusNotFound {
			t.Fatalf("request %s status = %d, body = %s", path, response.Code, response.Body.String())
		}
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
		{path: "/rest/getAlbumList.view" + auth + "&type=alphabeticalByName&size=500&offset=0", contains: []string{
			"<albumList>", `<album id="album_test"`, `title="Album"`,
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
		{path: "/rest/getRandomSongs.view" + auth + "&size=1&genre=Rock&fromYear=2020&toYear=2030", contains: []string{
			"<randomSongs>", `<song id="` + track.ID + `"`, `genre="Rock"`,
		}},
		{path: "/rest/getSong.view" + auth + "&id=" + track.ID, contains: []string{
			`<song id="` + track.ID + `"`,
		}},
		{path: "/rest/search3.view" + auth + "&query=artist&artistCount=20&albumCount=20&songCount=20", contains: []string{
			"<searchResult3>", `<artist id="artist_test"`, `<album id="album_test"`, `<song id="` + track.ID + `"`,
		}},
		{path: "/rest/search2.view" + auth + "&query=artist&artistCount=20&albumCount=20&songCount=20", contains: []string{
			"<searchResult2>", `<artist id="artist_test"`, `<album id="album_test"`, `<song id="` + track.ID + `"`,
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

func TestPlaylistLifecycle(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	auth := "?u=alice&p=secret&f=json"
	request := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}

	created := request("/rest/createPlaylist" + auth + "&name=Roadtrip&songId=" + track.ID)
	if created.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	var createdBody struct {
		Response struct {
			Playlist playlist `json:"playlist"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatal(err)
	}
	id := createdBody.Response.Playlist.ID
	if id == "" || createdBody.Response.Playlist.SongCount != 1 || len(createdBody.Response.Playlist.Entries) != 1 {
		t.Fatalf("created playlist = %#v", createdBody.Response.Playlist)
	}

	listed := request("/rest/getPlaylists" + auth)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"id":"`+id+`"`) ||
		!strings.Contains(listed.Body.String(), `"songCount":1`) {
		t.Fatalf("list status = %d, body = %s", listed.Code, listed.Body.String())
	}

	updated := request("/rest/updatePlaylist" + auth + "&playlistId=" + id +
		"&name=Renamed&comment=Mobile&public=true&songIndexToRemove=0&songIdToAdd=" + track.ID)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"status":"ok"`) {
		t.Fatalf("update status = %d, body = %s", updated.Code, updated.Body.String())
	}

	replaced := request("/rest/createPlaylist" + auth + "&playlistId=" + id +
		"&songId=" + track.ID + "&songId=" + track.ID)
	if replaced.Code != http.StatusOK || !strings.Contains(replaced.Body.String(), `"songCount":2`) {
		t.Fatalf("replace status = %d, body = %s", replaced.Code, replaced.Body.String())
	}

	detail := request("/rest/getPlaylist" + auth + "&id=" + id)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"name":"Renamed"`) ||
		!strings.Contains(detail.Body.String(), `"comment":"Mobile"`) ||
		!strings.Contains(detail.Body.String(), `"public":true`) ||
		strings.Count(detail.Body.String(), `"id":"`+track.ID+`"`) != 2 {
		t.Fatalf("detail status = %d, body = %s", detail.Code, detail.Body.String())
	}
	xmlDetail := request("/rest/getPlaylist?u=alice&p=secret&id=" + id)
	if xmlDetail.Code != http.StatusOK || !strings.Contains(xmlDetail.Body.String(), `<playlist id="`+id+`"`) ||
		strings.Count(xmlDetail.Body.String(), `<entry id="`+track.ID+`"`) != 2 {
		t.Fatalf("XML detail status = %d, body = %s", xmlDetail.Code, xmlDetail.Body.String())
	}

	deleted := request("/rest/deletePlaylist" + auth + "&id=" + id)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
	missing := request("/rest/getPlaylist" + auth + "&id=" + id)
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"code":70`) {
		t.Fatalf("missing status = %d, body = %s", missing.Code, missing.Body.String())
	}
}

func TestPlaylistEndpointsValidateMutations(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	auth := "?u=alice&p=secret&f=json"
	tests := []struct {
		path     string
		contains string
	}{
		{path: "/rest/createPlaylist" + auth, contains: "name"},
		{path: "/rest/getPlaylist" + auth, contains: "id"},
		{path: "/rest/updatePlaylist" + auth, contains: "playlistId"},
		{path: "/rest/deletePlaylist" + auth, contains: "id"},
		{path: "/rest/createPlaylist" + auth + "&name=List&songId=missing", contains: "not found"},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code < 400 || !strings.Contains(strings.ToLower(response.Body.String()), strings.ToLower(test.contains)) {
			t.Fatalf("path = %s, status = %d, body = %s", test.path, response.Code, response.Body.String())
		}
	}

	created := httptest.NewRecorder()
	handler.ServeHTTP(created, httptest.NewRequest(http.MethodGet,
		"/rest/createPlaylist"+auth+"&name=List&songId="+track.ID, nil))
	var body struct {
		Response struct {
			Playlist playlist `json:"playlist"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"&public=maybe", "&songIndexToRemove=-1", "&songIndexToRemove=2"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
			"/rest/updatePlaylist"+auth+"&playlistId="+body.Response.Playlist.ID+suffix, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("suffix = %s, status = %d, body = %s", suffix, response.Code, response.Body.String())
		}
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

func TestRandomSongsValidatesAndFilters(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	auth := "?u=alice&p=secret&f=json"
	tests := []struct {
		name       string
		parameters string
		status     int
		contains   string
	}{
		{name: "default", status: http.StatusOK, contains: `"id":"` + track.ID + `"`},
		{name: "invalid size", parameters: "&size=-1", status: http.StatusBadRequest, contains: "size"},
		{name: "invalid from year", parameters: "&fromYear=no", status: http.StatusBadRequest, contains: "fromYear"},
		{name: "invalid to year", parameters: "&toYear=-1", status: http.StatusBadRequest, contains: "toYear"},
		{name: "genre", parameters: "&genre=rock", status: http.StatusOK, contains: `"id":"` + track.ID + `"`},
		{name: "different genre", parameters: "&genre=Pop", status: http.StatusOK, contains: `"song":[]`},
		{name: "year range", parameters: "&fromYear=2020&toYear=2030", status: http.StatusOK, contains: `"id":"` + track.ID + `"`},
		{name: "outside year", parameters: "&toYear=2020", status: http.StatusOK, contains: `"song":[]`},
		{name: "zero size", parameters: "&size=0", status: http.StatusOK, contains: `"song":[]`},
		{name: "unknown folder", parameters: "&musicFolderId=other", status: http.StatusOK, contains: `"song":[]`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
				"/rest/getRandomSongs"+auth+test.parameters, nil))
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

func TestSearch3AddsPermittedSoulseekResultsOnlyWhenPlayed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { catalog.Close() })
	localTrack := domain.Track{
		ID: "local-track", Title: "Shared Song", Artist: "Artist", ArtistID: "local-artist",
		Album: "Album", AlbumID: "local-album", AlbumArtist: "Artist", Suffix: "mp3", ContentType: "audio/mpeg",
	}
	localRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(localRoot, "shared.mp3"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ReplaceProviderTracks(ctx, local.Name, []domain.TrackSource{{
		Track: localTrack, Ref: domain.SourceRef{Provider: local.Name, Key: "shared.mp3"},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	remoteTrack := domain.Track{
		ID: "remote-track", Title: "Remote Song", Artist: "Remote Artist", ArtistID: "remote-artist",
		Album: "Remote Album", AlbumID: "remote-album", AlbumArtist: "Remote Artist",
		Suffix: "flac", ContentType: "audio/flac", Size: 12,
	}
	provider := &discoveryProviderStub{media: []byte("remote-media"), results: []domain.TrackSource{
		{
			Track: domain.Track{ID: "duplicate", Title: "Shared Song", Artist: "Artist", Album: "Album"},
			Ref:   domain.SourceRef{Provider: "soulseek", Key: "duplicate"}, DiscoveredAt: time.Now().UTC(),
		},
		{
			Track: remoteTrack, Ref: domain.SourceRef{Provider: "soulseek", Key: "remote"},
			DiscoveredAt: time.Now().UTC(),
		},
	}}
	discovery := services.NewDiscoveryService(provider, catalog)
	localProvider, err := local.New(localRoot)
	if err != nil {
		t.Fatal(err)
	}
	streams := services.NewStreamingService(catalog, localProvider, provider)

	withoutPermission := NewHandlerWithAuthenticatorAndDiscovery(
		catalog, streams, nil, permissionAuthenticator{user: domain.User{Role: domain.UserRoleUser}}, discovery,
	)
	response := httptest.NewRecorder()
	withoutPermission.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/rest/search3?u=listener&p=secret&f=json&query=Song&artistCount=0&albumCount=0&songCount=3", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), remoteTrack.ID) {
		t.Fatalf("search without permission status = %d, body = %s", response.Code, response.Body.String())
	}

	withPermission := NewHandlerWithAuthenticatorAndDiscovery(
		catalog, streams, nil, permissionAuthenticator{user: domain.User{
			Role: domain.UserRoleUser, Permissions: []domain.Permission{domain.PermissionSoulseekClient},
		}}, discovery,
	)
	response = httptest.NewRecorder()
	withPermission.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/rest/search3?u=listener&p=secret&f=json&query=Song&artistCount=0&albumCount=0&songCount=3", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, remoteTrack.ID) || strings.Contains(body, "duplicate") ||
		strings.Index(body, localTrack.ID) > strings.Index(body, remoteTrack.ID) {
		t.Fatalf("permitted search status = %d, body = %s", response.Code, body)
	}
	if _, err := catalog.Track(ctx, remoteTrack.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("remote track persisted before playback: %v", err)
	}
	getSong := httptest.NewRecorder()
	withPermission.ServeHTTP(getSong, httptest.NewRequest(http.MethodGet,
		"/rest/getSong?u=listener&p=secret&f=json&id="+remoteTrack.ID, nil))
	if getSong.Code != http.StatusOK || !strings.Contains(getSong.Body.String(), remoteTrack.ID) {
		t.Fatalf("virtual getSong status = %d, body = %s", getSong.Code, getSong.Body.String())
	}
	stream := httptest.NewRecorder()
	withPermission.ServeHTTP(stream, httptest.NewRequest(http.MethodGet,
		"/rest/stream?u=listener&p=secret&id="+remoteTrack.ID, nil))
	if stream.Code != http.StatusOK || stream.Body.String() != "remote-media" {
		t.Fatalf("remote stream status = %d, body = %q", stream.Code, stream.Body.String())
	}
	if persisted, err := catalog.Track(ctx, remoteTrack.ID); err != nil || persisted.Title != remoteTrack.Title {
		t.Fatalf("persisted remote track = %#v, %v", persisted, err)
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

func TestLegacyDiscoveryEndpointsUseLegacyJSONShapes(t *testing.T) {
	t.Parallel()

	handler, track := newTestHandler(t)
	for _, test := range []struct {
		path     string
		contains []string
	}{
		{
			path:     "/rest/getAlbumList?u=alice&p=secret&f=json&type=alphabeticalByName",
			contains: []string{`"albumList"`, `"album":[`, `"isDir":true`, `"id":"album_test"`},
		},
		{
			path:     "/rest/search2?u=alice&p=secret&f=json&query=artist",
			contains: []string{`"searchResult2"`, `"artist":[`, `"album":[`, `"song":[`, `"id":"` + track.ID + `"`},
		},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("path = %s, status = %d, body = %s", test.path, response.Code, response.Body.String())
		}
		for _, expected := range test.contains {
			if !strings.Contains(response.Body.String(), expected) {
				t.Errorf("path = %s, body does not contain %q: %s", test.path, expected, response.Body.String())
			}
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
