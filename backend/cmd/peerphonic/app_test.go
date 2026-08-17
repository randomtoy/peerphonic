package main

import (
	"bytes"
	"context"
	"encoding/json"
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
		CacheDir: t.TempDir(), Username: "admin", Password: "secret", Scan: true,
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

	indexes := httptest.NewRecorder()
	app.handler.ServeHTTP(indexes, httptest.NewRequest(http.MethodGet,
		"/rest/getIndexes?u=admin&p=secret&f=json", nil))
	if indexes.Code != http.StatusOK || !strings.Contains(indexes.Body.String(), "Artist") {
		t.Fatalf("indexes status = %d, body = %s", indexes.Code, indexes.Body.String())
	}

	trackID := domain.StableID("track", "local", "Artist/Album/Song.mp3")
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
