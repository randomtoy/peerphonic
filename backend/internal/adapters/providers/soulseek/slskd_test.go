package soulseek

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestSlskdProviderStatusAuthenticatesWithAPIKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/base/api/v0/session" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("X-API-Key") != "0123456789abcdef" {
			t.Errorf("API key = %q", request.Header.Get("X-API-Key"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"authenticated":true}`))
	}))
	defer server.Close()
	client, err := NewSlskd(server.URL+"/base", "0123456789abcdef", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	status := client.ProviderStatus(context.Background())
	if !status.Configured || !status.Reachable || !status.Authenticated || status.Provider != Name {
		t.Fatalf("ProviderStatus() = %#v", status)
	}
}

func TestSlskdProviderStatusReportsRejectedAndUnavailableConnections(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	client, err := NewSlskd(server.URL, "wrong-api-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	status := client.ProviderStatus(context.Background())
	if !status.Reachable || status.Authenticated || status.Message == "" {
		t.Fatalf("rejected ProviderStatus() = %#v", status)
	}
	server.Close()
	status = client.ProviderStatus(context.Background())
	if status.Reachable || status.Authenticated || status.Message == "" {
		t.Fatalf("unavailable ProviderStatus() = %#v", status)
	}
}

func TestNewSlskdValidatesEndpoint(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"", "slskd:5030", "ftp://slskd:5030"} {
		if _, err := NewSlskd(endpoint, "", time.Second); err == nil {
			t.Fatalf("NewSlskd(%q) error = nil", endpoint)
		}
	}
}

func TestSlskdSearchMapsAudioResultsAndCleansUp(t *testing.T) {
	t.Parallel()

	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-API-Key") != "0123456789abcdef" {
			t.Errorf("API key = %q", request.Header.Get("X-API-Key"))
		}
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/base/api/v0/searches":
			var payload struct {
				SearchText string `json:"searchText"`
				FileLimit  int    `json:"fileLimit"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload.SearchText != "Massive Attack" || payload.FileLimit != 12 {
				t.Errorf("payload = %#v", payload)
			}
			_, _ = writer.Write([]byte(`{"id":"11111111-1111-1111-1111-111111111111","isComplete":false}`))
		case request.Method == http.MethodGet && request.URL.Path == "/base/api/v0/searches/11111111-1111-1111-1111-111111111111":
			if request.URL.Query().Get("includeResponses") != "true" {
				t.Errorf("includeResponses = %q", request.URL.Query().Get("includeResponses"))
			}
			_, _ = writer.Write([]byte(`{
				"id":"11111111-1111-1111-1111-111111111111","isComplete":true,
				"responses":[{
					"username":"peer-one","hasFreeUploadSlot":true,"queueLength":2,"uploadSpeed":1048576,
					"files":[
						{"filename":"Massive Attack\\Mezzanine\\01 Angel.mp3","extension":"mp3","size":1200,"length":360,"bitRate":320},
						{"filename":"Massive Attack\\Mezzanine\\cover.jpg","extension":"jpg","size":400}
					],
					"lockedFiles":[{"filename":"Massive Attack\\Mezzanine\\02 Risingson.flac","extension":"flac","size":2400}]
				}]
			}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/base/api/v0/searches/11111111-1111-1111-1111-111111111111":
			deleted.Store(true)
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewSlskd(server.URL+"/base", "0123456789abcdef", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	results, err := client.Search(context.Background(), domain.SearchQuery{Text: " Massive Attack ", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	first := results[0]
	if first.Track.Title != "01 Angel" || first.Track.Duration != 6*time.Minute || first.Track.BitRate != 320 ||
		first.Track.Artist != "Massive Attack" || first.Track.Album != "Mezzanine" ||
		first.Track.ContentType != "audio/mpeg" || first.Track.ArtistID == "" || first.Track.AlbumID == "" ||
		first.DisplayPath != "Massive Attack/Mezzanine/01 Angel.mp3" || first.Availability.Peer != "peer-one" ||
		!first.Availability.FreeUploadSlot || first.Availability.QueueLength != 2 || first.Availability.UploadSpeed != 1048576 ||
		first.Ref.Provider != Name || first.Ref.Key == "" || strings.Contains(first.Ref.Key, "peer-one") {
		t.Fatalf("first result = %#v", first)
	}
	if !results[1].Availability.RequiresApproval {
		t.Fatalf("locked result = %#v", results[1])
	}
	if !deleted.Load() {
		t.Fatal("completed slskd search was not deleted")
	}
}

func TestSlskdSearchValidatesQueryAndReportsUnavailableProvider(t *testing.T) {
	t.Parallel()

	client, err := NewSlskd("http://127.0.0.1:1", "key", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Search(context.Background(), domain.SearchQuery{Text: "ab"}); err == nil {
		t.Fatal("short query error = nil")
	}
	_, err = client.Search(context.Background(), domain.SearchQuery{Text: "valid query"})
	if !errors.Is(err, ports.ErrSourceUnavailable) {
		t.Fatalf("unavailable error = %v", err)
	}
}

func TestSlskdResolveStreamsGrowingFileAndReusesCompletedDownload(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	downloadsDir := filepath.Join(root, "downloads")
	incompleteDir := filepath.Join(root, "incomplete")
	remote := remoteFileRef{
		Peer: "peer-one", Path: `@@abcde\Massive Attack\Mezzanine\01 Angel.mp3`, Size: 10,
	}
	trackID := domain.StableID(Name, remote.Peer, remote.Path, strconv.FormatInt(remote.Size, 10))
	incompletePath := filepath.Join(
		incompleteDir, "peer-one", "Massive Attack", "Mezzanine", "01 Angel.mp3",
	)
	finalPath := filepath.Join(downloadsDir, trackID, "01 Angel.mp3")
	var enqueueCalls atomic.Int32
	var pollCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v0/transfers/downloads/batches":
			enqueueCalls.Add(1)
			var payload struct {
				Username string `json:"username"`
				Files    []struct {
					Filename string `json:"filename"`
					Size     int64  `json:"size"`
				} `json:"files"`
				Options struct {
					Destination string `json:"destination"`
				} `json:"options"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload.Username != remote.Peer || len(payload.Files) != 1 ||
				payload.Files[0].Filename != remote.Path || payload.Files[0].Size != remote.Size ||
				payload.Options.Destination != trackID {
				t.Errorf("enqueue payload = %#v", payload)
			}
			_, _ = writer.Write([]byte(`{"batch":{"id":"batch-1","transfers":[{"id":"transfer-1","state":"Queued, Locally"}]},"failures":[]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v0/transfers/downloads/batches/batch-1":
			poll := pollCalls.Add(1)
			if poll == 1 {
				if err := os.MkdirAll(filepath.Dir(incompletePath), 0o750); err != nil {
					t.Error(err)
				}
				if err := os.WriteFile(incompletePath, []byte("audio"), 0o640); err != nil {
					t.Error(err)
				}
				_, _ = writer.Write([]byte(`{"id":"batch-1","transfers":[{"id":"transfer-1","state":"InProgress, Remotely","bytesTransferred":5}]}`))
				return
			}
			file, err := os.OpenFile(incompletePath, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Error(err)
			} else {
				_, _ = file.WriteString("-data")
				_ = file.Close()
			}
			if err := os.MkdirAll(filepath.Dir(finalPath), 0o750); err != nil {
				t.Error(err)
			}
			if err := os.Rename(incompletePath, finalPath); err != nil {
				t.Error(err)
			}
			_, _ = writer.Write([]byte(`{"id":"batch-1","transfers":[{"id":"transfer-1","state":"Completed, Succeeded, Remotely","bytesTransferred":10}]}`))
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetMediaDirectories(downloadsDir, incompleteDir); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(remote)
	ref := domain.SourceRef{Provider: Name, Key: base64.RawURLEncoding.EncodeToString(encoded)}
	resolved, err := client.Resolve(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Resolve(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Content.Close()
	if end, err := resolved.Content.Seek(0, io.SeekEnd); err != nil || end != remote.Size {
		t.Fatalf("Seek(end) = %d, %v", end, err)
	}
	if _, err := resolved.Content.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resolved.Content)
	if err != nil {
		t.Fatal(err)
	}
	_ = resolved.Content.Close()
	if string(data) != "audio-data" {
		t.Fatalf("streamed data = %q", data)
	}

	cached, err := client.Resolve(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	cachedData, err := io.ReadAll(cached.Content)
	_ = cached.Content.Close()
	if err != nil || string(cachedData) != "audio-data" || enqueueCalls.Load() != 1 {
		t.Fatalf("cached data = %q, err = %v, enqueue calls = %d", cachedData, err, enqueueCalls.Load())
	}
}

func TestSlskdResolveRequiresMediaDirectoriesAndValidReference(t *testing.T) {
	t.Parallel()

	client, err := NewSlskd("http://slskd:5030", "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Resolve(context.Background(), domain.SourceRef{Provider: Name, Key: "invalid"})
	if !errors.Is(err, ports.ErrSourceUnavailable) {
		t.Fatalf("unconfigured Resolve() error = %v", err)
	}
	if err := client.SetMediaDirectories(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resolve(context.Background(), domain.SourceRef{Provider: Name, Key: "invalid"}); err == nil {
		t.Fatal("invalid Resolve() error = nil")
	}
}

func TestSlskdPathSanitizationMatchesRemoteRoots(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]string{
		`C:\Music\Album\song.mp3`:       filepath.Join("Music", "Album"),
		`\\server\Music\Album\song.mp3`: filepath.Join("Music", "Album"),
		`@@abcde\Music\Album\song.mp3`:  filepath.Join("Music", "Album"),
		`Music\..\Album\song.mp3`:       filepath.Join("Music", "_", "Album"),
	} {
		if got := sanitizedRemoteDirectory(path); got != want {
			t.Errorf("sanitizedRemoteDirectory(%q) = %q, want %q", path, got, want)
		}
	}
}
