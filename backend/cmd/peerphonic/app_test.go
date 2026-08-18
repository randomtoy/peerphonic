package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/config"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/tcolgate/mp3"
)

func TestLocalFileToOpenSubsonicStream(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mediaPath := filepath.Join(root, "Artist", "Album", "Song.mp3")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	media := bytes.Repeat(mp3.SilentBytes, 10)
	if err := os.WriteFile(mediaPath, media, 0o600); err != nil {
		t.Fatal(err)
	}
	cover := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	if err := os.WriteFile(filepath.Join(filepath.Dir(mediaPath), "cover.jpg"), cover, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		MusicDir: root, Database: filepath.Join(t.TempDir(), "peerphonic.db"),
		CacheDir: t.TempDir(), CacheSizeBytes: 1024, TorrentDir: t.TempDir(),
		Username: "admin", Password: "secret", Scan: true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app, err := buildApplication(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("buildApplication() error = %v", err)
	}
	defer app.Close()
	rootResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(rootResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if rootResponse.Code != http.StatusOK {
		t.Fatalf("root status = %d, body = %s", rootResponse.Code, rootResponse.Body.String())
	}
	readyResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(readyResponse, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
	if readyResponse.Code != http.StatusOK || !strings.Contains(readyResponse.Body.String(), `"status":"ready"`) {
		t.Fatalf("ready status = %d, body = %s", readyResponse.Code, readyResponse.Body.String())
	}
	createUser := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(
		`{"username":"listener","password":"listener-password","role":"user"}`,
	))
	createUser.SetBasicAuth("admin", "secret")
	createUser.Header.Set("Content-Type", "application/json")
	createUserResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(createUserResponse, createUser)
	if createUserResponse.Code != http.StatusCreated ||
		!strings.Contains(createUserResponse.Body.String(), `"username":"listener"`) {
		t.Fatalf("create user status = %d, body = %s", createUserResponse.Code, createUserResponse.Body.String())
	}
	salt := "client-salt"
	token := fmt.Sprintf("%x", md5.Sum([]byte("listener-password"+salt)))
	userPing := httptest.NewRecorder()
	app.handler.ServeHTTP(userPing, httptest.NewRequest(http.MethodGet,
		"/rest/ping?u=listener&s="+salt+"&t="+token+"&f=json", nil))
	if userPing.Code != http.StatusOK || !strings.Contains(userPing.Body.String(), `"status":"ok"`) {
		t.Fatalf("new user ping status = %d, body = %s", userPing.Code, userPing.Body.String())
	}
	nonAdminUsers := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	nonAdminUsers.SetBasicAuth("listener", "listener-password")
	nonAdminUsersResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(nonAdminUsersResponse, nonAdminUsers)
	if nonAdminUsersResponse.Code != http.StatusForbidden {
		t.Fatalf("non-admin users status = %d, want %d", nonAdminUsersResponse.Code, http.StatusForbidden)
	}
	permissionRequest := httptest.NewRequest(
		http.MethodPut, "/api/v1/users/listener/permissions",
		strings.NewReader(`{"permissions":["dashboard.access","monitoring.view"]}`),
	)
	permissionRequest.SetBasicAuth("admin", "secret")
	permissionRequest.Header.Set("Content-Type", "application/json")
	permissionResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(permissionResponse, permissionRequest)
	if permissionResponse.Code != http.StatusOK ||
		!strings.Contains(permissionResponse.Body.String(), `"dashboard.access"`) {
		t.Fatalf("update permissions status = %d, body = %s", permissionResponse.Code, permissionResponse.Body.String())
	}
	listenerSessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	listenerSessionRequest.SetBasicAuth("listener", "listener-password")
	listenerSession := httptest.NewRecorder()
	app.handler.ServeHTTP(listenerSession, listenerSessionRequest)
	if listenerSession.Code != http.StatusOK ||
		!strings.Contains(listenerSession.Body.String(), `"monitoring.view"`) {
		t.Fatalf("listener session status = %d, body = %s", listenerSession.Code, listenerSession.Body.String())
	}
	listenerCacheRequest := httptest.NewRequest(http.MethodGet, "/api/v1/cache/status", nil)
	listenerCacheRequest.SetBasicAuth("listener", "listener-password")
	listenerCache := httptest.NewRecorder()
	app.handler.ServeHTTP(listenerCache, listenerCacheRequest)
	if listenerCache.Code != http.StatusOK {
		t.Fatalf("delegated cache status = %d, body = %s", listenerCache.Code, listenerCache.Body.String())
	}
	listenerSourcesRequest := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	listenerSourcesRequest.SetBasicAuth("listener", "listener-password")
	listenerSources := httptest.NewRecorder()
	app.handler.ServeHTTP(listenerSources, listenerSourcesRequest)
	if listenerSources.Code != http.StatusForbidden {
		t.Fatalf("undelegated source status = %d, want %d", listenerSources.Code, http.StatusForbidden)
	}
	listenerScanRequest := httptest.NewRequest(http.MethodGet, "/api/v1/library/scan", nil)
	listenerScanRequest.SetBasicAuth("listener", "listener-password")
	listenerScan := httptest.NewRecorder()
	app.handler.ServeHTTP(listenerScan, listenerScanRequest)
	if listenerScan.Code != http.StatusForbidden {
		t.Fatalf("undelegated scan status = %d, want %d", listenerScan.Code, http.StatusForbidden)
	}
	listenerSettingsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/settings/transfers", nil)
	listenerSettingsRequest.SetBasicAuth("listener", "listener-password")
	listenerSettings := httptest.NewRecorder()
	app.handler.ServeHTTP(listenerSettings, listenerSettingsRequest)
	if listenerSettings.Code != http.StatusForbidden {
		t.Fatalf("undelegated settings status = %d, want %d", listenerSettings.Code, http.StatusForbidden)
	}
	listenerSoulseekRequest := httptest.NewRequest(http.MethodGet, "/api/v1/providers/soulseek/status", nil)
	listenerSoulseekRequest.SetBasicAuth("listener", "listener-password")
	listenerSoulseek := httptest.NewRecorder()
	app.handler.ServeHTTP(listenerSoulseek, listenerSoulseekRequest)
	if listenerSoulseek.Code != http.StatusForbidden {
		t.Fatalf("undelegated soulseek status = %d, want %d", listenerSoulseek.Code, http.StatusForbidden)
	}
	cacheResponse := httptest.NewRecorder()
	cacheRequest := httptest.NewRequest(http.MethodGet, "/api/v1/cache/status", nil)
	cacheRequest.SetBasicAuth("admin", "secret")
	app.handler.ServeHTTP(cacheResponse, cacheRequest)
	if cacheResponse.Code != http.StatusOK || !strings.Contains(cacheResponse.Body.String(), `"capacityBytes":1024`) {
		t.Fatalf("cache status = %d, body = %s", cacheResponse.Code, cacheResponse.Body.String())
	}

	indexes := httptest.NewRecorder()
	app.handler.ServeHTTP(indexes, httptest.NewRequest(http.MethodGet,
		"/rest/getIndexes?u=admin&p=secret&f=json", nil))
	if indexes.Code != http.StatusOK || !strings.Contains(indexes.Body.String(), "Artist") {
		t.Fatalf("indexes status = %d, body = %s", indexes.Code, indexes.Body.String())
	}

	trackID := domain.StableID("track", "local", "Artist/Album/Song.mp3")
	listenerQueue := httptest.NewRecorder()
	app.handler.ServeHTTP(listenerQueue, httptest.NewRequest(http.MethodGet,
		"/rest/savePlayQueue?u=listener&p=listener-password&f=json&id="+trackID+"&current="+trackID, nil))
	if listenerQueue.Code != http.StatusOK {
		t.Fatalf("listener queue status = %d, body = %s", listenerQueue.Code, listenerQueue.Body.String())
	}
	adminQueue := httptest.NewRecorder()
	app.handler.ServeHTTP(adminQueue, httptest.NewRequest(http.MethodGet,
		"/rest/getPlayQueue?u=admin&p=secret&f=json", nil))
	if adminQueue.Code != http.StatusOK || !strings.Contains(adminQueue.Body.String(), `"entry":[]`) {
		t.Fatalf("admin queue leaked listener state: status = %d, body = %s", adminQueue.Code, adminQueue.Body.String())
	}
	stream := httptest.NewRecorder()
	app.handler.ServeHTTP(stream, httptest.NewRequest(http.MethodGet,
		"/rest/stream?u=admin&p=secret&id="+trackID, nil))
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status = %d, body = %s", stream.Code, stream.Body.String())
	}
	if !bytes.Equal(stream.Body.Bytes(), media) {
		t.Fatalf("streamed %d bytes, want %d", stream.Body.Len(), len(media))
	}

	artistID := domain.StableID("artist", "artist")
	albumID := domain.StableID("album", artistID, "album")
	albumResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(albumResponse, httptest.NewRequest(http.MethodGet,
		"/rest/getAlbum?u=admin&p=secret&f=json&id="+albumID, nil))
	var payload struct {
		Response struct {
			Album struct {
				CoverArt string `json:"coverArt"`
			} `json:"album"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(albumResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Response.Album.CoverArt == "" {
		t.Fatalf("album body has no coverArt: %s", albumResponse.Body.String())
	}
	coverResponse := httptest.NewRecorder()
	app.handler.ServeHTTP(coverResponse, httptest.NewRequest(http.MethodGet,
		"/rest/getCoverArt?u=admin&p=secret&id="+payload.Response.Album.CoverArt, nil))
	if coverResponse.Code != http.StatusOK || !bytes.Equal(coverResponse.Body.Bytes(), cover) {
		t.Fatalf("cover status = %d, body size = %d", coverResponse.Code, coverResponse.Body.Len())
	}
}
