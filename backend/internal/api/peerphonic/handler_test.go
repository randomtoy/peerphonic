package peerphonic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type cacheStatusStub struct{}

type sourceImporterStub struct {
	contents string
}

type transferMonitorStub struct{}

type downloadMonitorStub struct {
	cancelID string
	retryID  string
}

type scanControllerStub struct {
	status scanner.Status
	starts int
}

type transferSettingsManagerStub struct {
	limits domain.TransferLimits
}

type providerStatusMonitorStub struct{ status domain.ProviderStatus }

type sourceSearcherStub struct {
	query         domain.SearchQuery
	results       []domain.TrackSource
	err           error
	addedID       string
	added         domain.Track
	addErr        error
	collectionID  string
	collection    domain.SourceCollection
	collectionErr error
}

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

func (*downloadMonitorStub) TrackDownloads(context.Context) ([]domain.TrackDownload, error) {
	return []domain.TrackDownload{{
		ID: "download-1", Provider: "torrent", SourceID: "source-1", TrackID: "track-1",
		Name: "Song.mp3", State: domain.DownloadStateDownloading,
		CompletedBytes: 50, TotalBytes: 100,
	}}, nil
}

func (s *downloadMonitorStub) CancelTrackDownload(_ context.Context, id string) error {
	if id == "missing" {
		return ports.ErrNotFound
	}
	s.cancelID = id
	return nil
}

func (s *downloadMonitorStub) RetryTrackDownload(_ context.Context, id string) error {
	if id == "missing" {
		return ports.ErrNotFound
	}
	s.retryID = id
	return nil
}

func (s *scanControllerStub) Start() bool {
	s.starts++
	if s.status.Scanning {
		return false
	}
	s.status.Scanning = true
	return true
}

func (s *scanControllerStub) Status() scanner.Status { return s.status }

func (s *transferSettingsManagerStub) Limits(
	context.Context, domain.User,
) (domain.TransferLimits, error) {
	return s.limits, nil
}

func (s *transferSettingsManagerStub) UpdateLimits(
	_ context.Context, _ domain.User, limits domain.TransferLimits,
) (domain.TransferLimits, error) {
	s.limits = limits
	return limits, nil
}

func (s providerStatusMonitorStub) ProviderStatus(context.Context) domain.ProviderStatus {
	return s.status
}

func (s *sourceSearcherStub) Name() string { return "soulseek" }

func (s *sourceSearcherStub) Search(
	_ context.Context, query domain.SearchQuery,
) ([]domain.TrackSource, error) {
	s.query = query
	return s.results, s.err
}

func (s *sourceSearcherStub) Add(_ context.Context, id string) (domain.Track, error) {
	s.addedID = id
	return s.added, s.addErr
}

func (s *sourceSearcherStub) AddCollection(
	_ context.Context, id string,
) (domain.SourceCollection, error) {
	s.collectionID = id
	return s.collection, s.collectionErr
}

func (s *sourceSearcherStub) PreviewCollection(
	_ context.Context, id string,
) (domain.SourceCollection, error) {
	s.collectionID = id
	return s.collection, s.collectionErr
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

	request := httptest.NewRequest(http.MethodGet, "/api/v1/cache/status", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	NewHandler(cacheStatusStub{}, nil, nil, nil, nil, nil, "alice", "secret").ServeHTTP(response, request)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"capacityBytes":100`) ||
		!strings.Contains(response.Body.String(), `"pinnedEntries":1`) ||
		!strings.Contains(response.Body.String(), `"partialEntries":1`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestLibraryScanStatusAndStartRequireAuthentication(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, time.August, 18, 7, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	scans := &scanControllerStub{status: scanner.Status{
		Count: 507, LastStartedAt: startedAt, LastFinishedAt: finishedAt,
	}}
	handler := NewHandler(nil, nil, nil, nil, nil, nil, "alice", "secret", scans)
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/library/scan", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/library/scan", nil)
	statusRequest.SetBasicAuth("alice", "secret")
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK ||
		!strings.Contains(statusResponse.Body.String(), `"tracks":507`) ||
		!strings.Contains(statusResponse.Body.String(), `"lastFinishedAt":"2026-08-18T07:01:00Z"`) {
		t.Fatalf("status = %d, body = %s", statusResponse.Code, statusResponse.Body.String())
	}

	startRequest := httptest.NewRequest(http.MethodPost, "/api/v1/library/scan", nil)
	startRequest.SetBasicAuth("alice", "secret")
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusAccepted || scans.starts != 1 ||
		!strings.Contains(startResponse.Body.String(), `"scanning":true`) {
		t.Fatalf("status = %d, starts = %d, body = %s", startResponse.Code, scans.starts, startResponse.Body.String())
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "/api/v1/library/scan", nil)
	secondRequest.SetBasicAuth("alice", "secret")
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK || scans.starts != 2 {
		t.Fatalf("second status = %d, starts = %d", secondResponse.Code, scans.starts)
	}
}

func TestTransferSettingsRequireAuthenticationAndUpdate(t *testing.T) {
	t.Parallel()

	settings := &transferSettingsManagerStub{limits: domain.TransferLimits{
		UploadBytesPerSecond: 1024, DownloadBytesPerSecond: 2048,
	}}
	handler := newHandler(
		nil, nil, nil, nil, nil, nil,
		fixedAuthenticator{username: "alice", password: "secret"}, nil, nil, nil, settings, nil,
	)
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/settings/transfers", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/settings/transfers", nil)
	getRequest.SetBasicAuth("alice", "secret")
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK ||
		!strings.Contains(getResponse.Body.String(), `"downloadLimitBytesPerSecond":2048`) {
		t.Fatalf("status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}

	putRequest := httptest.NewRequest(http.MethodPut, "/api/v1/settings/transfers", strings.NewReader(
		`{"uploadLimitBytesPerSecond":4096,"downloadLimitBytesPerSecond":8192}`,
	))
	putRequest.SetBasicAuth("alice", "secret")
	putResponse := httptest.NewRecorder()
	handler.ServeHTTP(putResponse, putRequest)
	if putResponse.Code != http.StatusOK || settings.limits.UploadBytesPerSecond != 4096 ||
		settings.limits.DownloadBytesPerSecond != 8192 {
		t.Fatalf("status = %d, limits = %#v, body = %s", putResponse.Code, settings.limits, putResponse.Body.String())
	}
}

func TestSoulseekProviderStatus(t *testing.T) {
	t.Parallel()

	monitor := providerStatusMonitorStub{status: domain.ProviderStatus{
		Provider: "soulseek", Configured: true, Reachable: true, Authenticated: true,
		Message: "slskd API is ready",
	}}
	handler := newHandler(
		nil, nil, nil, nil, nil, nil,
		fixedAuthenticator{username: "alice", password: "secret"}, nil, monitor, nil, nil, nil,
	)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/providers/soulseek/status", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"configured":true`) ||
		!strings.Contains(response.Body.String(), `"authenticated":true`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	disabled := NewHandler(nil, nil, nil, nil, nil, nil, "alice", "secret")
	disabledRequest := httptest.NewRequest(http.MethodGet, "/api/v1/providers/soulseek/status", nil)
	disabledRequest.SetBasicAuth("alice", "secret")
	disabledResponse := httptest.NewRecorder()
	disabled.ServeHTTP(disabledResponse, disabledRequest)
	if disabledResponse.Code != http.StatusOK ||
		!strings.Contains(disabledResponse.Body.String(), `"configured":false`) {
		t.Fatalf("disabled status = %d, body = %s", disabledResponse.Code, disabledResponse.Body.String())
	}
}

func TestSoulseekSearchRequiresPermissionAndReturnsGenericResults(t *testing.T) {
	t.Parallel()

	searcher := &sourceSearcherStub{results: []domain.TrackSource{
		{
			Track: domain.Track{
				ID: "soulseek_opaque", Title: "Angel", Artist: "Massive Attack", Album: "Mezzanine",
				Size: 12_000, Duration: 6 * time.Minute, BitRate: 320, Suffix: "mp3",
			},
			DisplayPath: "Massive Attack/Mezzanine/01 Angel.mp3",
			Availability: domain.SourceAvailability{
				Peer: "peer-one", UploadSpeed: 1_048_576, QueueLength: 2, FreeUploadSlot: true,
			},
		},
		{
			Track: domain.Track{
				ID: "soulseek_second", Title: "Risingson", Artist: "Massive Attack", Album: "Mezzanine",
				Size: 24_000, Duration: 5 * time.Minute, BitRate: 1000, Suffix: "flac",
			},
			DisplayPath: "Massive Attack/Mezzanine/02 Risingson.flac",
			Availability: domain.SourceAvailability{
				Peer: "peer-one", UploadSpeed: 1_048_576, QueueLength: 2, FreeUploadSlot: true,
			},
		},
	}}
	handler := newHandler(
		nil, nil, nil, nil, nil, nil,
		fixedAuthenticator{username: "alice", password: "secret"}, nil, nil, searcher, nil, nil,
	)
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(
		http.MethodPost, "/api/v1/providers/soulseek/search", strings.NewReader(`{"query":"Massive Attack"}`),
	))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/providers/soulseek/search",
		strings.NewReader(`{"query":" Massive Attack ","limit":25}`),
	)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var payload sourceSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || searcher.query.Text != "Massive Attack" || searcher.query.Limit != 25 ||
		!strings.Contains(response.Body.String(), `"id":"soulseek_opaque"`) ||
		!strings.Contains(response.Body.String(), `"path":"Massive Attack/Mezzanine/01 Angel.mp3"`) ||
		!strings.Contains(response.Body.String(), `"uploadSpeedBytesPerSecond":1048576`) ||
		!strings.Contains(response.Body.String(), `"durationSeconds":360`) ||
		len(payload.Collections) != 1 || payload.Collections[0].Name != "Mezzanine" ||
		payload.Collections[0].MatchedTracks != 2 || payload.Collections[0].MatchedSize != 36_000 ||
		len(payload.Collections[0].Formats) != 2 || len(payload.Collections[0].Results) != 2 {
		t.Fatalf("status = %d, query = %#v, body = %s", response.Code, searcher.query, response.Body.String())
	}
}

func TestSoulseekSearchValidatesRequestAndRequiresConfiguredProvider(t *testing.T) {
	t.Parallel()

	handler := NewHandler(nil, nil, nil, nil, nil, nil, "alice", "secret")
	for _, payload := range []string{`{"query":"ab"}`, `{"query":"valid","limit":201}`, `{"query":"valid","extra":true}`} {
		request := httptest.NewRequest(
			http.MethodPost, "/api/v1/providers/soulseek/search", strings.NewReader(payload),
		)
		request.SetBasicAuth("alice", "secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("disabled status = %d, body = %s", response.Code, response.Body.String())
		}
	}

	searcher := &sourceSearcherStub{}
	configured := newHandler(
		nil, nil, nil, nil, nil, nil,
		fixedAuthenticator{username: "alice", password: "secret"}, nil, nil, searcher, nil, nil,
	)
	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/providers/soulseek/search", strings.NewReader(`{"query":"ab"}`),
	)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	configured.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSoulseekSearchGroupingKeepsPeersAndLockedFilesSeparate(t *testing.T) {
	t.Parallel()

	results := []sourceSearchResultResponse{
		{ID: "one", Path: `Artist\Album\01.mp3`, Peer: "peer-one"},
		{ID: "two", Path: "Artist/Album/02.flac", Peer: "peer-one"},
		{ID: "three", Path: "Artist/Album/01.mp3", Peer: "peer-two"},
		{ID: "locked", Path: "Artist/Album/03.mp3", Peer: "peer-one", RequiresApproval: true},
	}
	groups := groupSourceSearchResults(results)
	if len(groups) != 3 || groups[0].MatchedTracks != 2 || len(groups[0].Results) != 2 ||
		groups[1].Peer != "peer-two" || !groups[2].RequiresApproval {
		t.Fatalf("groupSourceSearchResults() = %#v", groups)
	}
}

func TestSoulseekTrackAddRequiresPermissionAndPersistsSelectedResult(t *testing.T) {
	t.Parallel()

	searcher := &sourceSearcherStub{added: domain.Track{
		ID: "soulseek_opaque", Title: "Angel", Artist: "Massive Attack", Album: "Mezzanine",
	}}
	handler := newHandler(
		nil, nil, nil, nil, nil, nil,
		fixedAuthenticator{username: "alice", password: "secret"}, nil, nil, searcher, nil, nil,
	)
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(
		http.MethodPost, "/api/v1/providers/soulseek/tracks/soulseek_opaque", nil,
	))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/providers/soulseek/tracks/soulseek_opaque", nil,
	)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || searcher.addedID != "soulseek_opaque" ||
		!strings.Contains(response.Body.String(), `"artist":"Massive Attack"`) ||
		!strings.Contains(response.Body.String(), `"album":"Mezzanine"`) {
		t.Fatalf("status = %d, added = %q, body = %s", response.Code, searcher.addedID, response.Body.String())
	}
}

func TestSoulseekTrackAddReportsExpiredResult(t *testing.T) {
	t.Parallel()

	searcher := &sourceSearcherStub{addErr: services.ErrDiscoveryResultNotFound}
	handler := newHandler(
		nil, nil, nil, nil, nil, nil,
		fixedAuthenticator{username: "alice", password: "secret"}, nil, nil, searcher, nil, nil,
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/providers/soulseek/tracks/missing", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSoulseekAlbumAddReturnsImportedCollection(t *testing.T) {
	t.Parallel()

	searcher := &sourceSearcherStub{collection: domain.SourceCollection{
		Name: "Mezzanine", Artist: "Massive Attack", CoverArtID: "remote-cover",
		Tracks: []domain.TrackSource{{}, {}, {}},
	}}
	handler := newHandler(
		nil, nil, nil, nil, nil, nil,
		fixedAuthenticator{username: "alice", password: "secret"}, nil, nil, searcher, nil, nil,
	)
	previewRequest := httptest.NewRequest(http.MethodGet, "/api/v1/providers/soulseek/albums/anchor", nil)
	previewRequest.SetBasicAuth("alice", "secret")
	previewResponse := httptest.NewRecorder()
	handler.ServeHTTP(previewResponse, previewRequest)
	if previewResponse.Code != http.StatusOK ||
		!strings.Contains(previewResponse.Body.String(), `"hasArtwork":true`) ||
		!strings.Contains(previewResponse.Body.String(), `"tracks":[`) {
		t.Fatalf("preview status = %d, body = %s", previewResponse.Code, previewResponse.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/providers/soulseek/albums/anchor", nil)
	request.SetBasicAuth("alice", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || searcher.collectionID != "anchor" ||
		!strings.Contains(response.Body.String(), `"name":"Mezzanine"`) ||
		!strings.Contains(response.Body.String(), `"tracks":3`) {
		t.Fatalf("status = %d, id = %q, body = %s", response.Code, searcher.collectionID, response.Body.String())
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

	handler := NewHandler(nil, nil, nil, nil, nil, &downloadMonitorStub{}, "alice", "secret")
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

func TestTrackDownloadActionsRequireAuthenticationAndRouteByID(t *testing.T) {
	t.Parallel()

	downloads := &downloadMonitorStub{}
	handler := NewHandler(nil, nil, nil, nil, nil, downloads, "alice", "secret")
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodDelete, "/api/v1/downloads/job-1", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	for method, path := range map[string]string{
		http.MethodDelete: "/api/v1/downloads/job-1",
		http.MethodPost:   "/api/v1/downloads/job-1/retry",
	} {
		request := httptest.NewRequest(method, path, nil)
		request.SetBasicAuth("alice", "secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s %s status = %d, body = %s", method, path, response.Code, response.Body.String())
		}
	}
	if downloads.cancelID != "job-1" || downloads.retryID != "job-1" {
		t.Fatalf("cancel = %q, retry = %q", downloads.cancelID, downloads.retryID)
	}

	missing := httptest.NewRequest(http.MethodDelete, "/api/v1/downloads/missing", nil)
	missing.SetBasicAuth("alice", "secret")
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missingResponse.Code)
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
