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

type transferMonitorStub struct{}

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
		Components: []domain.CacheUsage{{Name: "torrent", Entries: 2, PartialEntries: 1, Size: 75}},
	}, nil
}

func (transferMonitorStub) Transfers(context.Context) ([]domain.SourceTransfer, error) {
	return []domain.SourceTransfer{{
		Provider: "torrent", ID: "abc", Name: "Album", CompletedBytes: 75, TotalBytes: 100,
		DownloadedBytes: 80, UploadedBytes: 25, Peers: 4, ActivePeers: 2,
		DownloadLimit: 2048, UploadLimit: 1024,
		ConnectedSeeders: 1, ActiveStreams: 1, Seeding: true,
	}}, nil
}

func TestHealth(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRootIsReachableForClientDiscovery(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"service":"peerphonic"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCacheStatus(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(cacheStatusStub{}, nil, nil, "", "").ServeHTTP(response,
		httptest.NewRequest(http.MethodGet, "/api/v1/cache/status", nil))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"capacityBytes":100`) ||
		!strings.Contains(response.Body.String(), `"pinnedEntries":1`) ||
		!strings.Contains(response.Body.String(), `"partialEntries":1`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestTorrentImportRequiresBasicAuthentication(t *testing.T) {
	t.Parallel()

	importer := &sourceImporterStub{}
	handler := NewHandler(nil, importer, nil, "alice", "secret")
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

func TestTransferStatusRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler := NewHandler(nil, nil, transferMonitorStub{}, "alice", "secret")
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/transfers", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/transfers", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK ||
		!strings.Contains(body, `"provider":"torrent"`) ||
		!strings.Contains(body, `"completedBytes":75`) ||
		!strings.Contains(body, `"uploadedBytes":25`) ||
		!strings.Contains(body, `"downloadLimitBytesPerSecond":2048`) ||
		!strings.Contains(body, `"uploadLimitBytesPerSecond":1024`) ||
		!strings.Contains(body, `"activePeers":2`) ||
		!strings.Contains(body, `"activeStreams":1`) ||
		!strings.Contains(body, `"seeding":true`) {
		t.Fatalf("status = %d, body = %s", response.Code, body)
	}
}
