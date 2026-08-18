package soulseek

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
