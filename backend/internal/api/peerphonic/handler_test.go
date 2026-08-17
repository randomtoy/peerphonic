package peerphonic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type cacheStatusStub struct{}

type sourceImporterStub struct {
	contents string
}

func (s *sourceImporterStub) Import(_ context.Context, source io.Reader) (ports.SourceImportResult, error) {
	contents, err := io.ReadAll(source)
	if err != nil {
		return ports.SourceImportResult{}, err
	}
	s.contents = string(contents)
	return ports.SourceImportResult{SourceID: "abc", Name: "Album", Tracks: 2}, nil
}

func (cacheStatusStub) Stats(context.Context) (domain.CacheStats, error) {
	return domain.CacheStats{
		Capacity: 100, Entries: 2, Size: 75, PinnedEntries: 1, PinnedSize: 50,
	}, nil
}

func TestHealth(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRootIsReachableForClientDiscovery(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"service":"peerphonic"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCacheStatus(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(cacheStatusStub{}, nil, "", "").ServeHTTP(response,
		httptest.NewRequest(http.MethodGet, "/api/v1/cache/status", nil))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"capacityBytes":100`) ||
		!strings.Contains(response.Body.String(), `"pinnedEntries":1`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestTorrentImportRequiresBasicAuthentication(t *testing.T) {
	t.Parallel()

	importer := &sourceImporterStub{}
	handler := NewHandler(nil, importer, "alice", "secret")
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized,
		httptest.NewRequest(http.MethodPost, "/api/v1/torrents", strings.NewReader("torrent")))
	if unauthorized.Code != http.StatusUnauthorized || importer.contents != "" {
		t.Fatalf("unauthorized status = %d, imported = %q", unauthorized.Code, importer.contents)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", strings.NewReader("torrent"))
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || importer.contents != "torrent" ||
		!strings.Contains(response.Body.String(), `"id":"abc"`) ||
		!strings.Contains(response.Body.String(), `"tracks":2`) {
		t.Fatalf("status = %d, body = %s, imported = %q", response.Code, response.Body.String(), importer.contents)
	}
}
