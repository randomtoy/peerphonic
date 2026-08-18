package soulseek

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
				SearchText    string `json:"searchText"`
				SearchTimeout int    `json:"searchTimeout"`
				FileLimit     int    `json:"fileLimit"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload.SearchText != "Massive Attack" || payload.SearchTimeout != 5_000 || payload.FileLimit != 12 {
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
	client.cleanupDelay = 10 * time.Millisecond
	defer client.Close()
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
	deadline := time.Now().Add(time.Second)
	for !deleted.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !deleted.Load() {
		t.Fatal("completed slskd search was not deleted after cleanup delay")
	}
}

func TestSlskdSearchFallsBackToLongestTermAndFiltersResults(t *testing.T) {
	t.Parallel()

	var searches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v0/searches":
			var payload struct {
				SearchText string `json:"searchText"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			call := searches.Add(1)
			if call == 1 {
				if payload.SearchText != "linkin park" {
					t.Errorf("first search text = %q", payload.SearchText)
				}
				_, _ = writer.Write([]byte(`{"id":"exact","isComplete":false}`))
				return
			}
			if payload.SearchText != "linkin" {
				t.Errorf("fallback search text = %q", payload.SearchText)
			}
			_, _ = writer.Write([]byte(`{"id":"fallback","isComplete":false}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v0/searches/exact":
			_, _ = writer.Write([]byte(`{"id":"exact","isComplete":true,"responses":[]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v0/searches/fallback":
			_, _ = writer.Write([]byte(`{
				"id":"fallback","isComplete":true,
				"responses":[{"username":"peer","files":[
					{"filename":"Linkin Park\\Hybrid Theory\\01 Papercut.flac","extension":"flac","size":1200},
					{"filename":"Linkin Avenue\\Other Album\\01 Song.mp3","extension":"mp3","size":800}
				]}]
			}`))
		case request.Method == http.MethodDelete:
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	results, err := client.Search(context.Background(), domain.SearchQuery{Text: "linkin park", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if searches.Load() != 2 {
		t.Fatalf("search requests = %d, want 2", searches.Load())
	}
	if len(results) != 1 || results[0].Track.Artist != "Linkin Park" || results[0].Track.Title != "01 Papercut" {
		t.Fatalf("results = %#v", results)
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

func TestSlskdServerErrorsReportUnavailableProvider(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "The wait timed out after 5000 milliseconds", http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	err = client.doJSON(context.Background(), http.MethodGet, "/api/v0/session", nil, nil)
	if !errors.Is(err, ports.ErrSourceUnavailable) || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("doJSON() error = %v", err)
	}
}

func TestSoulseekSearchAcceptsCommonAudioFormats(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		"mp3": "audio/mpeg", "flac": "audio/flac", "ogg": "audio/ogg", "opus": "audio/ogg",
		"aac": "audio/aac", "m4a": "audio/mp4", "alac": "audio/mp4", "wav": "audio/wav",
		"aiff": "audio/aiff", "wma": "audio/x-ms-wma", "ape": "audio/ape", "wv": "audio/wavpack",
	}
	files := make([]slskdFile, 0, len(expected)+1)
	for suffix := range expected {
		files = append(files, slskdFile{Filename: `Artist\Album\track.` + suffix, Extension: suffix, Size: 100})
	}
	files = append(files, slskdFile{Filename: `Artist\Album\cover.jpg`, Extension: "jpg", Size: 100})
	results := mapSearchResults([]slskdSearchResponse{{Username: "peer", Files: files}}, 100)
	if len(results) != len(expected) {
		t.Fatalf("results = %d, want %d", len(results), len(expected))
	}
	for _, result := range results {
		if expected[result.Track.Suffix] != result.Track.ContentType {
			t.Errorf("format %q content type = %q", result.Track.Suffix, result.Track.ContentType)
		}
	}
}

func TestSlskdBrowseCollectionMapsDirectoryWithoutDownloadingAudio(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/api/v0/users/peer-one/directory" {
			http.Error(writer, "unexpected request", http.StatusNotFound)
			return
		}
		var payload struct {
			Directory string `json:"directory"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Directory != `Massive Attack\Mezzanine` {
			t.Errorf("directory = %q", payload.Directory)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{
			"name":"Massive Attack\\Mezzanine","files":[
				{"filename":"Massive Attack\\Mezzanine\\02 Risingson.flac","extension":"flac","size":2400,"length":289,"bitRate":1000},
				{"filename":"Massive Attack\\Mezzanine\\01 Angel.mp3","extension":"mp3","size":1200,"length":360,"bitRate":320},
				{"filename":"Massive Attack\\Mezzanine\\cover.jpg","extension":"jpg","size":400},
				{"filename":"Massive Attack\\Mezzanine\\notes.txt","extension":"txt","size":50}
			]
		}]`))
	}))
	defer server.Close()
	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	remote := remoteFileRef{Peer: "peer-one", Path: `Massive Attack\Mezzanine\01 Angel.mp3`, Size: 1200}
	encoded, _ := json.Marshal(remote)
	anchor := domain.TrackSource{
		Ref:          domain.SourceRef{Provider: Name, Key: base64.RawURLEncoding.EncodeToString(encoded)},
		Availability: domain.SourceAvailability{Peer: "peer-one", FreeUploadSlot: true},
	}
	collection, err := client.BrowseCollection(context.Background(), anchor)
	if err != nil {
		t.Fatal(err)
	}
	if collection.Name != "Mezzanine" || collection.Artist != "Massive Attack" || len(collection.Tracks) != 2 ||
		collection.Tracks[0].Track.Title != "01 Angel" || collection.Tracks[0].Track.TrackNumber != 1 ||
		collection.Tracks[1].Track.Title != "02 Risingson" || collection.CoverArtID == "" ||
		collection.Tracks[0].Track.CoverArtID != collection.CoverArtID || requests.Load() != 1 {
		t.Fatalf("BrowseCollection() = %#v, requests = %d", collection, requests.Load())
	}
	if !strings.HasPrefix(collection.CoverArtID, remoteArtworkPrefix) {
		t.Fatalf("cover ID = %q", collection.CoverArtID)
	}
}

func TestSlskdArtworkDownloadsOnceAndReusesCache(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	downloadsDir := filepath.Join(root, "downloads")
	remote := remoteFileRef{Peer: "peer-one", Path: `Artist\Album\cover.jpg`, Size: 10}
	encoded, _ := json.Marshal(remote)
	key := base64.RawURLEncoding.EncodeToString(encoded)
	destination := domain.StableID("soulseek-cover", key)
	finalPath := filepath.Join(downloadsDir, destination, "cover.jpg")
	var enqueueCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v0/transfers/downloads/batches":
			enqueueCalls.Add(1)
			var payload struct {
				Options struct {
					Destination string `json:"destination"`
				} `json:"options"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload.Options.Destination != destination {
				t.Errorf("destination = %q", payload.Options.Destination)
			}
			_, _ = writer.Write([]byte(`{"batch":{"id":"cover-batch","transfers":[{"id":"cover-transfer","state":"Queued, Locally"}]},"failures":[]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v0/transfers/downloads/batches/cover-batch":
			if err := os.MkdirAll(filepath.Dir(finalPath), 0o750); err != nil {
				t.Error(err)
			}
			if err := os.WriteFile(finalPath, []byte("image-data"), 0o640); err != nil {
				t.Error(err)
			}
			_, _ = writer.Write([]byte(`{"id":"cover-batch","transfers":[{"id":"cover-transfer","state":"Completed, Succeeded, Remotely","bytesTransferred":10}]}`))
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetMediaDirectories(downloadsDir, filepath.Join(root, "incomplete")); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for range 2 {
		resolved, err := client.OpenArtwork(context.Background(), remoteArtworkPrefix+key)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resolved.Content)
		_ = resolved.Content.Close()
		if err != nil || string(data) != "image-data" || resolved.ContentType != "image/jpeg" {
			t.Fatalf("artwork = %q, content type = %q, error = %v", data, resolved.ContentType, err)
		}
	}
	if enqueueCalls.Load() != 1 {
		t.Fatalf("enqueue calls = %d, want 1", enqueueCalls.Load())
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
	trackID := "catalog-track"
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
	completed := make(chan CompletedFile, 1)
	client.SetCompletedHandler(func(file CompletedFile) { completed <- file })
	encoded, _ := json.Marshal(remote)
	ref := domain.SourceRef{Provider: Name, Key: base64.RawURLEncoding.EncodeToString(encoded)}
	resolved, err := client.Resolve(context.Background(), trackID, ref)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Resolve(context.Background(), trackID, ref)
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
	select {
	case file := <-completed:
		if file.TrackID != trackID || file.Path != finalPath || file.Size != remote.Size {
			t.Fatalf("completed file = %#v", file)
		}
	case <-time.After(time.Second):
		t.Fatal("completed file callback was not invoked")
	}

	cached, err := client.Resolve(context.Background(), trackID, ref)
	if err != nil {
		t.Fatal(err)
	}
	cachedData, err := io.ReadAll(cached.Content)
	_ = cached.Content.Close()
	if err != nil || string(cachedData) != "audio-data" || enqueueCalls.Load() != 1 {
		t.Fatalf("cached data = %q, err = %v, enqueue calls = %d", cachedData, err, enqueueCalls.Load())
	}
	downloads, err := client.TrackDownloads(context.Background())
	if err != nil || len(downloads) != 1 || downloads[0].State != domain.DownloadStateCached ||
		downloads[0].CompletedBytes != remote.Size || downloads[0].Provider != Name || downloads[0].TrackID != trackID {
		t.Fatalf("TrackDownloads() = %#v, %v", downloads, err)
	}
	usage, err := client.CacheUsage(context.Background())
	if err != nil || usage.Name != Name || usage.Entries != 1 || usage.Size != remote.Size || usage.PartialEntries != 0 {
		t.Fatalf("CacheUsage() = %#v, %v", usage, err)
	}
}

func TestSlskdSerializesConcurrentEnqueuesForSamePeer(t *testing.T) {
	t.Parallel()

	var active atomic.Int32
	var maximum atomic.Int32
	var enqueueCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v0/transfers/downloads/batches":
			current := active.Add(1)
			defer active.Add(-1)
			for current > maximum.Load() && !maximum.CompareAndSwap(maximum.Load(), current) {
			}
			if current > 1 {
				http.Error(writer, "Only one concurrent operation is permitted", http.StatusTooManyRequests)
				return
			}
			time.Sleep(25 * time.Millisecond)
			call := enqueueCalls.Add(1)
			_, _ = writer.Write([]byte(fmt.Sprintf(
				`{"batch":{"id":"batch-%d","transfers":[{"id":"transfer-%d","state":"Queued, Locally"}]},"failures":[]}`,
				call, call,
			)))
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v0/transfers/downloads/batches/"):
			id := strings.TrimPrefix(request.URL.Path, "/api/v0/transfers/downloads/batches/")
			_, _ = writer.Write([]byte(fmt.Sprintf(
				`{"id":%q,"transfers":[{"id":"transfer","state":"Queued, Locally"}]}`, id,
			)))
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetMediaDirectories(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	const jobs = 4
	errors := make(chan error, jobs)
	var wait sync.WaitGroup
	for index := range jobs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			name := fmt.Sprintf("song-%d.mp3", index)
			remote := remoteFileRef{Peer: "same-peer", Path: `Album\` + name, Size: 10}
			_, enqueueErr := client.downloads.ensureDownload(
				context.Background(), fmt.Sprintf("key-%d", index), fmt.Sprintf("track-%d", index), name, remote,
			)
			errors <- enqueueErr
		}()
	}
	wait.Wait()
	close(errors)
	for enqueueErr := range errors {
		if enqueueErr != nil {
			t.Fatal(enqueueErr)
		}
	}
	if enqueueCalls.Load() != jobs || maximum.Load() != 1 {
		t.Fatalf("enqueue calls = %d, maximum concurrency = %d", enqueueCalls.Load(), maximum.Load())
	}
}

func TestSlskdDuplicateResolveWaitsForEnqueueResult(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	var startedOnce sync.Once
	var enqueueCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v0/transfers/downloads/batches" {
			http.Error(writer, "unexpected request", http.StatusNotFound)
			return
		}
		enqueueCalls.Add(1)
		startedOnce.Do(func() { close(started) })
		time.Sleep(1700 * time.Millisecond)
		http.Error(writer, "peer lookup failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewSlskd(server.URL, "key", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetMediaDirectories(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	remote := remoteFileRef{Peer: "peer-one", Path: `Album\song.mp3`, Size: 10}
	encoded, _ := json.Marshal(remote)
	ref := domain.SourceRef{Provider: Name, Key: base64.RawURLEncoding.EncodeToString(encoded)}

	firstResult := make(chan error, 1)
	go func() {
		_, resolveErr := client.Resolve(context.Background(), "logical-track", ref)
		firstResult <- resolveErr
	}()
	<-started
	if _, err := client.Resolve(context.Background(), "logical-track", ref); !errors.Is(err, ports.ErrSourceUnavailable) {
		t.Fatalf("duplicate Resolve() error = %v", err)
	}
	if err := <-firstResult; !errors.Is(err, ports.ErrSourceUnavailable) {
		t.Fatalf("first Resolve() error = %v", err)
	}
	if enqueueCalls.Load() != 1 {
		t.Fatalf("enqueue calls = %d, want 1", enqueueCalls.Load())
	}
}

func TestSlskdRetriesBusyPeerOperation(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "unexpected request", http.StatusNotFound)
			return
		}
		if calls.Add(1) == 1 {
			http.Error(writer, "Only one concurrent operation is permitted", http.StatusTooManyRequests)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		OK bool `json:"ok"`
	}
	if err := client.doPeerJSON(
		context.Background(), "peer-one", http.MethodPost, "/operation", nil, &response,
	); err != nil {
		t.Fatal(err)
	}
	if !response.OK || calls.Load() != 2 {
		t.Fatalf("response = %#v, calls = %d", response, calls.Load())
	}
}

func TestSlskdDownloadCanBeCancelledAndRetried(t *testing.T) {
	t.Parallel()

	var enqueueCalls atomic.Int32
	var cancelCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v0/transfers/downloads/batches":
			call := enqueueCalls.Add(1)
			_, _ = writer.Write([]byte(fmt.Sprintf(
				`{"batch":{"id":"batch-%d","transfers":[{"id":"transfer-%d","state":"Queued, Locally"}]},"failures":[]}`,
				call, call,
			)))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v0/transfers/downloads/batches/batch-1":
			_, _ = writer.Write([]byte(`{"id":"batch-1","transfers":[{"id":"transfer-1","state":"Queued, Locally"}]}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/api/v0/transfers/downloads/peer-one/transfer-1":
			cancelCalls.Add(1)
			writer.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v0/transfers/downloads/batches/batch-2":
			_, _ = writer.Write([]byte(`{"id":"batch-2","transfers":[{"id":"transfer-2","state":"Completed, Errored, Remotely","exception":"peer disconnected"}]}`))
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetMediaDirectories(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	remote := remoteFileRef{Peer: "peer-one", Path: `Album\song.mp3`, Size: 10}
	encoded, _ := json.Marshal(remote)
	ref := domain.SourceRef{Provider: Name, Key: base64.RawURLEncoding.EncodeToString(encoded)}
	resolved, err := client.Resolve(context.Background(), "", ref)
	if err != nil {
		t.Fatal(err)
	}
	downloads, err := client.TrackDownloads(context.Background())
	if err != nil || len(downloads) != 1 {
		t.Fatalf("TrackDownloads() = %#v, %v", downloads, err)
	}
	id := downloads[0].ID
	if err := client.CancelTrackDownload(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.Content.Read(make([]byte, 1)); err == nil {
		t.Fatal("cancelled stream read error = nil")
	}
	_ = resolved.Content.Close()
	downloads, _ = client.TrackDownloads(context.Background())
	if downloads[0].State != domain.DownloadStateCancelled || cancelCalls.Load() != 1 {
		t.Fatalf("cancelled download = %#v, calls = %d", downloads[0], cancelCalls.Load())
	}
	if err := client.RetryTrackDownload(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		downloads, err = client.TrackDownloads(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(downloads) == 1 && downloads[0].State == domain.DownloadStateFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("retried download = %#v", downloads)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if enqueueCalls.Load() != 2 || !strings.Contains(downloads[0].Error, "peer disconnected") {
		t.Fatalf("retry calls = %d, download = %#v", enqueueCalls.Load(), downloads[0])
	}
}

func TestSlskdResolveRejectsFailedTransferBeforeOpeningStream(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v0/transfers/downloads/batches":
			_, _ = writer.Write([]byte(`{"batch":{"id":"batch-1","transfers":[{"id":"transfer-1","state":"Queued, Locally"}]},"failures":[]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v0/transfers/downloads/batches/batch-1":
			_, _ = writer.Write([]byte(`{"id":"batch-1","transfers":[{"id":"transfer-1","state":"Completed, Rejected, Remotely","exception":"File not shared."}]}`))
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SetMediaDirectories(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	remote := remoteFileRef{Peer: "stale-peer", Path: `Album\song.mp3`, Size: 10}
	encoded, _ := json.Marshal(remote)
	_, err = client.Resolve(context.Background(), "", domain.SourceRef{
		Provider: Name, Key: base64.RawURLEncoding.EncodeToString(encoded),
	})
	if !errors.Is(err, ports.ErrSourceUnavailable) || !strings.Contains(err.Error(), "File not shared") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestSlskdCacheEvictionUsesLRUAndProtectsActiveStreams(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	downloadsDir := filepath.Join(root, "downloads")
	client, err := NewSlskd("http://slskd:5030", "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SetMediaDirectories(downloadsDir, filepath.Join(root, "incomplete")); err != nil {
		t.Fatal(err)
	}

	type cachedTrack struct {
		remote remoteFileRef
		key    string
		path   string
		reader ports.ReadSeekCloser
	}
	makeCached := func(name, contents string) cachedTrack {
		t.Helper()
		remote := remoteFileRef{Peer: "peer-one", Path: `Album\` + name, Size: int64(len(contents))}
		encoded, err := json.Marshal(remote)
		if err != nil {
			t.Fatal(err)
		}
		key := base64.RawURLEncoding.EncodeToString(encoded)
		trackID := domain.StableID(Name, remote.Peer, remote.Path, strconv.FormatInt(remote.Size, 10))
		path := filepath.Join(downloadsDir, trackID, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o640); err != nil {
			t.Fatal(err)
		}
		resolved, err := client.Resolve(context.Background(), "", domain.SourceRef{Provider: Name, Key: key})
		if err != nil {
			t.Fatal(err)
		}
		return cachedTrack{remote: remote, key: key, path: path, reader: resolved.Content}
	}

	oldestActive := makeCached("active.mp3", "active-data")
	oldInactive := makeCached("old.mp3", "old-data")
	newInactive := makeCached("new.mp3", "new-data")
	if err := oldInactive.reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := newInactive.reader.Close(); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	for _, item := range []struct {
		track cachedTrack
		age   time.Duration
	}{
		{track: oldestActive, age: 3 * time.Hour},
		{track: oldInactive, age: 2 * time.Hour},
		{track: newInactive, age: time.Hour},
	} {
		id := domain.StableID("download", Name, item.track.key)
		job := client.downloads.records[id]
		if job == nil {
			t.Fatalf("missing cached record %q", id)
		}
		job.mu.Lock()
		job.download.UpdatedAt = now.Add(-item.age)
		job.mu.Unlock()
	}

	freed, err := client.Evict(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if freed != oldInactive.remote.Size {
		t.Fatalf("first eviction freed %d bytes, want %d", freed, oldInactive.remote.Size)
	}
	if _, err := os.Stat(oldInactive.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old inactive cache file still exists: %v", err)
	}
	if _, err := os.Stat(oldestActive.path); err != nil {
		t.Fatalf("active cache file was evicted: %v", err)
	}

	if err := oldestActive.reader.Close(); err != nil {
		t.Fatal(err)
	}
	freed, err = client.Evict(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if freed != oldestActive.remote.Size {
		t.Fatalf("second eviction freed %d bytes, want %d", freed, oldestActive.remote.Size)
	}
	if _, err := os.Stat(oldestActive.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed oldest cache file still exists: %v", err)
	}
	if _, err := os.Stat(newInactive.path); err != nil {
		t.Fatalf("newest cache file was evicted: %v", err)
	}

	downloads, err := client.TrackDownloads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	states := make(map[string]domain.DownloadState, len(downloads))
	for _, download := range downloads {
		states[download.Name] = download.State
	}
	if states["active.mp3"] != domain.DownloadStateEvicted ||
		states["old.mp3"] != domain.DownloadStateEvicted ||
		states["new.mp3"] != domain.DownloadStateCached {
		t.Fatalf("download states = %#v", states)
	}
}

func TestSlskdDownloadResumesMonitoringAfterClientRestart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	downloadsDir := filepath.Join(root, "downloads")
	incompleteDir := filepath.Join(root, "incomplete")
	remote := remoteFileRef{Peer: "peer-one", Path: `Album\song.mp3`, Size: 10}
	trackID := "logical-track"
	finalPath := filepath.Join(downloadsDir, trackID, "song.mp3")
	var enqueueCalls atomic.Int32
	var complete atomic.Bool
	var writeFinal sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v0/transfers/downloads/batches":
			enqueueCalls.Add(1)
			_, _ = writer.Write([]byte(`{"batch":{"id":"batch-1","transfers":[{"id":"transfer-1","state":"Queued, Locally"}]},"failures":[]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v0/transfers/downloads/batches/batch-1":
			if complete.Load() {
				writeFinal.Do(func() {
					if err := os.MkdirAll(filepath.Dir(finalPath), 0o750); err != nil {
						t.Error(err)
					}
					if err := os.WriteFile(finalPath, []byte("audio-data"), 0o640); err != nil {
						t.Error(err)
					}
				})
				_, _ = writer.Write([]byte(`{"id":"batch-1","transfers":[{"id":"transfer-1","state":"Completed, Succeeded, Remotely","bytesTransferred":10}]}`))
				return
			}
			_, _ = writer.Write([]byte(`{"id":"batch-1","transfers":[{"id":"transfer-1","state":"Queued, Locally"}]}`))
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	encoded, _ := json.Marshal(remote)
	ref := domain.SourceRef{Provider: Name, Key: base64.RawURLEncoding.EncodeToString(encoded)}
	first, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SetMediaDirectories(downloadsDir, incompleteDir); err != nil {
		t.Fatal(err)
	}
	resolved, err := first.Resolve(context.Background(), trackID, ref)
	if err != nil {
		t.Fatal(err)
	}
	_ = resolved.Content.Close()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := NewSlskd(server.URL, "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.SetMediaDirectories(downloadsDir, incompleteDir); err != nil {
		t.Fatal(err)
	}
	downloads, err := second.TrackDownloads(context.Background())
	if err != nil || len(downloads) != 1 || downloads[0].State != domain.DownloadStateQueued {
		t.Fatalf("restored TrackDownloads() = %#v, %v", downloads, err)
	}
	complete.Store(true)
	if err := second.ResumeTrackDownloads(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		downloads, err = second.TrackDownloads(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(downloads) == 1 && downloads[0].State == domain.DownloadStateCached {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resumed TrackDownloads() = %#v", downloads)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if enqueueCalls.Load() != 1 {
		t.Fatalf("enqueue calls = %d, want 1", enqueueCalls.Load())
	}
}

func TestSlskdResolveRequiresMediaDirectoriesAndValidReference(t *testing.T) {
	t.Parallel()

	client, err := NewSlskd("http://slskd:5030", "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Resolve(context.Background(), "", domain.SourceRef{Provider: Name, Key: "invalid"})
	if !errors.Is(err, ports.ErrSourceUnavailable) {
		t.Fatalf("unconfigured Resolve() error = %v", err)
	}
	if err := client.SetMediaDirectories(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resolve(context.Background(), "", domain.SourceRef{Provider: Name, Key: "invalid"}); err == nil {
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

func TestSlskdResumeReportsInvalidPersistedJobWithoutBlockingStartup(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	downloadsDir := filepath.Join(root, "downloads")
	incompleteDir := filepath.Join(root, "incomplete")
	jobsDir := filepath.Join(root, "jobs")
	if err := os.MkdirAll(jobsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobsDir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := NewSlskd("http://slskd:5030", "key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SetMediaDirectories(downloadsDir, incompleteDir); err != nil {
		t.Fatalf("SetMediaDirectories() error = %v", err)
	}
	if err := client.ResumeTrackDownloads(context.Background()); err == nil {
		t.Fatal("ResumeTrackDownloads() error = nil")
	}
}
