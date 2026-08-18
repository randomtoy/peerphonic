package peerphonic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type cacheStatusStub struct{}

type sourceImporterStub struct {
	contents string
}

type transferMonitorStub struct{}

type downloadMonitorStub struct{}

type uriImporterStub struct {
	uri   string
	items []domain.SourceImport
}

type sourceManagerStub struct {
	sources         []domain.ManagedSource
	pausedID        string
	resumedID       string
	pinnedID        string
	pinned          bool
	removedID       string
	removedData     bool
	managementError error
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

func (downloadMonitorStub) TrackDownloads(context.Context) ([]domain.TrackDownload, error) {
	return []domain.TrackDownload{{
		ID: "download-1", Provider: "torrent", SourceID: "source-1", TrackID: "track-1",
		Name: "Song.mp3", State: domain.DownloadStateDownloading,
		CompletedBytes: 50, TotalBytes: 100,
	}}, nil
}

func (s *uriImporterStub) ImportURI(_ context.Context, uri string) (domain.SourceImport, error) {
	s.uri = uri
	return domain.SourceImport{
		ID: "job-1", Provider: "torrent", SourceID: "abc", Name: "Album",
		State: domain.SourceImportStateFetching, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}, nil
}

func (s *uriImporterStub) SourceImports(context.Context) ([]domain.SourceImport, error) {
	return s.items, nil
}

func (s *sourceManagerStub) ManagedSources(context.Context) ([]domain.ManagedSource, error) {
	return s.sources, s.managementError
}

func (s *sourceManagerStub) PauseSource(_ context.Context, id string) error {
	s.pausedID = id
	return s.managementError
}

func (s *sourceManagerStub) ResumeSource(_ context.Context, id string) error {
	s.resumedID = id
	return s.managementError
}

func (s *sourceManagerStub) PinSource(_ context.Context, id string, pinned bool) error {
	s.pinnedID = id
	s.pinned = pinned
	return s.managementError
}

func (s *sourceManagerStub) RemoveSource(_ context.Context, id string, deleteData bool) error {
	s.removedID = id
	s.removedData = deleteData
	return s.managementError
}

func TestHealth(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, nil, nil, nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRootIsReachableForClientDiscovery(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, nil, nil, nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"service":"peerphonic"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(nil, nil, nil, nil, nil, nil, "", "").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCacheStatus(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(cacheStatusStub{}, nil, nil, nil, nil, nil, "", "").ServeHTTP(response,
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
	handler := NewHandler(nil, importer, nil, nil, nil, nil, "alice", "secret")
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

func TestMagnetImportIsAsynchronousAndListed(t *testing.T) {
	t.Parallel()

	importer := &uriImporterStub{items: []domain.SourceImport{{
		ID: "job-1", Provider: "torrent", SourceID: "abc", Name: "Album", Tracks: 4,
		State: domain.SourceImportStateReady, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}}}
	handler := NewHandler(nil, nil, importer, nil, nil, nil, "alice", "secret")
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(
		http.MethodPost, "/api/v1/torrents/magnet", strings.NewReader(`{"magnet":"magnet:?xt=urn:btih:abc"}`),
	))
	if unauthorized.Code != http.StatusUnauthorized || importer.uri != "" {
		t.Fatalf("unauthorized status = %d, URI = %q", unauthorized.Code, importer.uri)
	}

	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/torrents/magnet", strings.NewReader(`{"magnet":"magnet:?xt=urn:btih:abc"}`),
	)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || importer.uri == "" ||
		!strings.Contains(response.Body.String(), `"state":"fetching_metadata"`) {
		t.Fatalf("status = %d, body = %s, URI = %q", response.Code, response.Body.String(), importer.uri)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/imports", nil)
	request.SetBasicAuth("alice", "secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"tracks":4`) ||
		!strings.Contains(response.Body.String(), `"state":"ready"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestTransferStatusRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler := NewHandler(nil, nil, nil, nil, transferMonitorStub{}, nil, "alice", "secret")
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

func TestTrackDownloadStatusRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler := NewHandler(nil, nil, nil, nil, nil, downloadMonitorStub{}, "alice", "secret")
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/downloads", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/downloads", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"id":"download-1"`) ||
		!strings.Contains(body, `"state":"downloading"`) ||
		!strings.Contains(body, `"completedBytes":50`) || !strings.Contains(body, `"totalBytes":100`) {
		t.Fatalf("status = %d, body = %s", response.Code, body)
	}
}

func TestTorrentManagementRequiresAuthenticationAndReturnsSources(t *testing.T) {
	t.Parallel()

	manager := &sourceManagerStub{sources: []domain.ManagedSource{{
		Provider: "torrent", ID: "abc", Name: "Album", Tracks: 12,
		Attached: true, Paused: false, Pinned: true,
	}}}
	handler := NewHandler(nil, nil, nil, manager, nil, nil, "alice", "secret")
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"name":"Album"`) ||
		!strings.Contains(body, `"tracks":12`) || !strings.Contains(body, `"pinned":true`) {
		t.Fatalf("status = %d, body = %s", response.Code, body)
	}
}

func TestTorrentManagementActions(t *testing.T) {
	t.Parallel()

	manager := &sourceManagerStub{}
	handler := NewHandler(nil, nil, nil, manager, nil, nil, "alice", "secret")
	for _, test := range []struct {
		path string
		want func() bool
	}{
		{path: "/api/v1/torrents/source-a/pause", want: func() bool { return manager.pausedID == "source-a" }},
		{path: "/api/v1/torrents/source-a/resume", want: func() bool { return manager.resumedID == "source-a" }},
		{path: "/api/v1/torrents/source-a/pin", want: func() bool { return manager.pinnedID == "source-a" && manager.pinned }},
		{path: "/api/v1/torrents/source-a/unpin", want: func() bool { return manager.pinnedID == "source-a" && !manager.pinned }},
	} {
		request := httptest.NewRequest(http.MethodPost, test.path, nil)
		request.SetBasicAuth("alice", "secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || !test.want() {
			t.Fatalf("POST %s status = %d, manager = %#v", test.path, response.Code, manager)
		}
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/torrents/source-a?deleteData=true", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || manager.removedID != "source-a" || !manager.removedData {
		t.Fatalf("DELETE status = %d, manager = %#v", response.Code, manager)
	}
}

func TestTorrentManagementReportsConflicts(t *testing.T) {
	t.Parallel()

	manager := &sourceManagerStub{managementError: ports.ErrSourceBusy}
	handler := NewHandler(nil, nil, nil, manager, nil, nil, "alice", "secret")
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/torrents/source-a", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
}
