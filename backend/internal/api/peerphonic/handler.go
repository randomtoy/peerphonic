package peerphonic

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type cacheStatus interface {
	Stats(ctx context.Context) (domain.CacheStats, error)
}

type scanController interface {
	Start() bool
	Status() scanner.Status
}

type transferResponse struct {
	Provider         string `json:"provider"`
	ID               string `json:"id"`
	Name             string `json:"name"`
	CompletedBytes   int64  `json:"completedBytes"`
	TotalBytes       int64  `json:"totalBytes"`
	DownloadedBytes  int64  `json:"downloadedBytes"`
	UploadedBytes    int64  `json:"uploadedBytes"`
	DownloadLimit    int64  `json:"downloadLimitBytesPerSecond"`
	UploadLimit      int64  `json:"uploadLimitBytesPerSecond"`
	Peers            int    `json:"peers"`
	ActivePeers      int    `json:"activePeers"`
	ConnectedSeeders int    `json:"connectedSeeders"`
	ActiveStreams    int    `json:"activeStreams"`
	Seeding          bool   `json:"seeding"`
}

type transfersResponse struct {
	Transfers []transferResponse `json:"transfers"`
}

type trackDownloadResponse struct {
	ID             string               `json:"id"`
	Provider       string               `json:"provider"`
	SourceID       string               `json:"sourceId"`
	TrackID        string               `json:"trackId,omitempty"`
	Name           string               `json:"name"`
	State          domain.DownloadState `json:"state"`
	CompletedBytes int64                `json:"completedBytes"`
	TotalBytes     int64                `json:"totalBytes"`
	Error          string               `json:"error,omitempty"`
	StartedAt      string               `json:"startedAt"`
	UpdatedAt      string               `json:"updatedAt"`
}

type trackDownloadsResponse struct {
	Downloads []trackDownloadResponse `json:"downloads"`
}

type managedSourceResponse struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Tracks   int    `json:"tracks"`
	Attached bool   `json:"attached"`
	Paused   bool   `json:"paused"`
	Pinned   bool   `json:"pinned"`
}

type managedSourcesResponse struct {
	Sources []managedSourceResponse `json:"sources"`
}

type sourceImportResponse struct {
	ID        string                   `json:"id"`
	Provider  string                   `json:"provider"`
	Name      string                   `json:"name,omitempty"`
	SourceID  string                   `json:"sourceId,omitempty"`
	Tracks    int                      `json:"tracks"`
	State     domain.SourceImportState `json:"state"`
	Error     string                   `json:"error,omitempty"`
	CreatedAt string                   `json:"createdAt"`
	UpdatedAt string                   `json:"updatedAt"`
}

type sourceImportsResponse struct {
	Imports []sourceImportResponse `json:"imports"`
}

type userResponse struct {
	Username    string              `json:"username"`
	Role        domain.UserRole     `json:"role"`
	Permissions []domain.Permission `json:"permissions"`
	CreatedAt   string              `json:"createdAt"`
	UpdatedAt   string              `json:"updatedAt"`
}

type usersResponse struct {
	Users []userResponse `json:"users"`
}

type cacheComponentResponse struct {
	Name           string `json:"name"`
	SizeBytes      int64  `json:"sizeBytes"`
	Entries        int    `json:"entries"`
	PartialEntries int    `json:"partialEntries"`
}

type cacheStatusResponse struct {
	CapacityBytes   int64                    `json:"capacityBytes"`
	SizeBytes       int64                    `json:"sizeBytes"`
	Entries         int                      `json:"entries"`
	PinnedSizeBytes int64                    `json:"pinnedSizeBytes"`
	PinnedEntries   int                      `json:"pinnedEntries"`
	Components      []cacheComponentResponse `json:"components"`
}

type libraryScanResponse struct {
	Scanning       bool   `json:"scanning"`
	Tracks         int    `json:"tracks"`
	LastError      string `json:"lastError,omitempty"`
	LastStartedAt  string `json:"lastStartedAt,omitempty"`
	LastFinishedAt string `json:"lastFinishedAt,omitempty"`
}

type transferSettingsResponse struct {
	UploadLimitBytesPerSecond   int64 `json:"uploadLimitBytesPerSecond"`
	DownloadLimitBytesPerSecond int64 `json:"downloadLimitBytesPerSecond"`
}

type providerStatusResponse struct {
	Provider      string `json:"provider"`
	Configured    bool   `json:"configured"`
	Reachable     bool   `json:"reachable"`
	Authenticated bool   `json:"authenticated"`
	Message       string `json:"message"`
}

type sourceSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type sourceSearchResultResponse struct {
	ID                        string `json:"id"`
	Title                     string `json:"title"`
	Artist                    string `json:"artist,omitempty"`
	Album                     string `json:"album,omitempty"`
	Path                      string `json:"path"`
	Size                      int64  `json:"size"`
	DurationSeconds           int64  `json:"durationSeconds,omitempty"`
	BitRate                   int    `json:"bitRate,omitempty"`
	Suffix                    string `json:"suffix,omitempty"`
	Peer                      string `json:"peer"`
	UploadSpeedBytesPerSecond int64  `json:"uploadSpeedBytesPerSecond"`
	QueueLength               int64  `json:"queueLength"`
	FreeUploadSlot            bool   `json:"freeUploadSlot"`
	RequiresApproval          bool   `json:"requiresApproval"`
}

type sourceSearchResponse struct {
	Query       string                           `json:"query"`
	Results     []sourceSearchResultResponse     `json:"results"`
	Collections []sourceSearchCollectionResponse `json:"collections"`
}

type sourceSearchCollectionResponse struct {
	ID                        string                       `json:"id"`
	Name                      string                       `json:"name"`
	Artist                    string                       `json:"artist"`
	Path                      string                       `json:"path"`
	Peer                      string                       `json:"peer"`
	MatchedTracks             int                          `json:"matchedTracks"`
	MatchedSize               int64                        `json:"matchedSize"`
	Formats                   []string                     `json:"formats"`
	UploadSpeedBytesPerSecond int64                        `json:"uploadSpeedBytesPerSecond"`
	QueueLength               int64                        `json:"queueLength"`
	FreeUploadSlot            bool                         `json:"freeUploadSlot"`
	RequiresApproval          bool                         `json:"requiresApproval"`
	Results                   []sourceSearchResultResponse `json:"results"`
}

type sourceAdder interface {
	Add(ctx context.Context, id string) (domain.Track, error)
}

type sourceCollectionAdder interface {
	AddCollection(ctx context.Context, id string) (domain.SourceCollection, error)
	PreviewCollection(ctx context.Context, id string) (domain.SourceCollection, error)
}

type sourceAddResponse struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
}

type sourceCollectionAddResponse struct {
	Name   string `json:"name"`
	Artist string `json:"artist"`
	Tracks int    `json:"tracks"`
}

type sourceCollectionPreviewResponse struct {
	Name       string                                 `json:"name"`
	Artist     string                                 `json:"artist"`
	HasArtwork bool                                   `json:"hasArtwork"`
	Tracks     []sourceCollectionPreviewTrackResponse `json:"tracks"`
}

type sourceCollectionPreviewTrackResponse struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Size            int64  `json:"size"`
	DurationSeconds int64  `json:"durationSeconds,omitempty"`
	Suffix          string `json:"suffix,omitempty"`
}

type catalogArtistAliasRequest struct {
	TargetID string `json:"targetId"`
}

type catalogTrackPatchRequest struct {
	Title       *string `json:"title"`
	Artist      *string `json:"artist"`
	Album       *string `json:"album"`
	AlbumArtist *string `json:"albumArtist"`
	Genre       *string `json:"genre"`
	Year        *int    `json:"year"`
	TrackNumber *int    `json:"trackNumber"`
	DiscNumber  *int    `json:"discNumber"`
}

func NewHandler(
	cache cacheStatus,
	torrentImporter ports.SourceImporter,
	uriImporter ports.SourceURIImporter,
	sources ports.SourceManager,
	transfers ports.SourceTransferMonitor,
	downloads ports.TrackDownloadMonitor,
	username, password string,
	scans ...scanController,
) http.Handler {
	var scan scanController
	if len(scans) > 0 {
		scan = scans[0]
	}
	return newHandler(
		cache, torrentImporter, uriImporter, sources, transfers, downloads,
		fixedAuthenticator{username: username, password: password}, nil, nil, nil, nil, scan,
	)
}

func NewHandlerWithAuthenticator(
	cache cacheStatus,
	torrentImporter ports.SourceImporter,
	uriImporter ports.SourceURIImporter,
	sources ports.SourceManager,
	transfers ports.SourceTransferMonitor,
	downloads ports.TrackDownloadMonitor,
	authenticator ports.Authenticator,
	users ports.UserManager,
	providerStatus ports.ProviderStatusMonitor,
	providerSearch ports.SourceSearcher,
	settings ports.TransferSettingsManager,
	scans ...scanController,
) http.Handler {
	var scan scanController
	if len(scans) > 0 {
		scan = scans[0]
	}
	return newHandler(
		cache, torrentImporter, uriImporter, sources, transfers, downloads,
		authenticator, users, providerStatus, providerSearch, settings, scan,
	)
}

func NewHandlerWithAuthenticatorAndCatalog(
	cache cacheStatus,
	torrentImporter ports.SourceImporter,
	uriImporter ports.SourceURIImporter,
	sources ports.SourceManager,
	transfers ports.SourceTransferMonitor,
	downloads ports.TrackDownloadMonitor,
	authenticator ports.Authenticator,
	users ports.UserManager,
	providerStatus ports.ProviderStatusMonitor,
	providerSearch ports.SourceSearcher,
	settings ports.TransferSettingsManager,
	catalog ports.CatalogManager,
	scans ...scanController,
) http.Handler {
	var scan scanController
	if len(scans) > 0 {
		scan = scans[0]
	}
	return newHandlerWithCatalog(
		cache, torrentImporter, uriImporter, sources, transfers, downloads,
		authenticator, users, providerStatus, providerSearch, settings, catalog, scan,
	)
}

func newHandler(
	cache cacheStatus,
	torrentImporter ports.SourceImporter,
	uriImporter ports.SourceURIImporter,
	sources ports.SourceManager,
	transfers ports.SourceTransferMonitor,
	downloads ports.TrackDownloadMonitor,
	authenticator ports.Authenticator,
	users ports.UserManager,
	providerStatus ports.ProviderStatusMonitor,
	providerSearch ports.SourceSearcher,
	settings ports.TransferSettingsManager,
	scans scanController,
) http.Handler {
	return newHandlerWithCatalog(
		cache, torrentImporter, uriImporter, sources, transfers, downloads,
		authenticator, users, providerStatus, providerSearch, settings, nil, scans,
	)
}

func newHandlerWithCatalog(
	cache cacheStatus,
	torrentImporter ports.SourceImporter,
	uriImporter ports.SourceURIImporter,
	sources ports.SourceManager,
	transfers ports.SourceTransferMonitor,
	downloads ports.TrackDownloadMonitor,
	authenticator ports.Authenticator,
	users ports.UserManager,
	providerStatus ports.ProviderStatusMonitor,
	providerSearch ports.SourceSearcher,
	settings ports.TransferSettingsManager,
	catalog ports.CatalogManager,
	scans scanController,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"service": "peerphonic",
			"status":  "ok",
		})
	})
	mux.HandleFunc("GET /api/v1/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"status":  "ok",
			"service": "peerphonic",
		})
	})
	if catalog != nil {
		mux.HandleFunc("GET /api/v1/catalog/artists", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionCatalogManage); !ok {
				return
			}
			artists, err := catalog.Artists(request.Context())
			if err != nil {
				http.Error(writer, "read catalog artists", http.StatusInternalServerError)
				return
			}
			aliases, err := catalog.ArtistAliases(request.Context())
			if err != nil {
				http.Error(writer, "read artist aliases", http.StatusInternalServerError)
				return
			}
			writeJSON(writer, http.StatusOK, struct {
				Artists []domain.Artist      `json:"artists"`
				Aliases []domain.ArtistAlias `json:"aliases"`
			}{Artists: artists, Aliases: aliases})
		})
		mux.HandleFunc("PUT /api/v1/catalog/artist-aliases/{id}", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionCatalogManage); !ok {
				return
			}
			var payload catalogArtistAliasRequest
			if !decodeJSONRequest(writer, request, 16<<10, &payload) {
				return
			}
			alias, err := catalog.SetArtistAlias(request.Context(), request.PathValue("id"), payload.TargetID)
			if err != nil {
				writeCatalogManagementError(writer, err)
				return
			}
			writeJSON(writer, http.StatusOK, alias)
		})
		mux.HandleFunc("DELETE /api/v1/catalog/artist-aliases/{id}", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionCatalogManage); !ok {
				return
			}
			if err := catalog.DeleteArtistAlias(request.Context(), request.PathValue("id")); err != nil {
				writeCatalogManagementError(writer, err)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("GET /api/v1/catalog/tracks/{id}", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionCatalogManage); !ok {
				return
			}
			track, err := catalog.Track(request.Context(), request.PathValue("id"))
			if err != nil {
				writeCatalogManagementError(writer, err)
				return
			}
			writeJSON(writer, http.StatusOK, track)
		})
		mux.HandleFunc("PATCH /api/v1/catalog/tracks/{id}", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionCatalogManage); !ok {
				return
			}
			var payload catalogTrackPatchRequest
			if !decodeJSONRequest(writer, request, 32<<10, &payload) {
				return
			}
			track, err := catalog.UpdateTrackMetadata(request.Context(), request.PathValue("id"), domain.TrackMetadataPatch{
				Title: payload.Title, Artist: payload.Artist, Album: payload.Album,
				AlbumArtist: payload.AlbumArtist, Genre: payload.Genre, Year: payload.Year,
				TrackNumber: payload.TrackNumber, DiscNumber: payload.DiscNumber,
			})
			if err != nil {
				writeCatalogManagementError(writer, err)
				return
			}
			writeJSON(writer, http.StatusOK, track)
		})
	}
	if scans != nil {
		mux.HandleFunc("GET /api/v1/library/scan", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
				return
			}
			writeJSON(writer, http.StatusOK, newLibraryScanResponse(scans.Status()))
		})
		mux.HandleFunc("POST /api/v1/library/scan", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
				return
			}
			status := http.StatusAccepted
			if !scans.Start() {
				status = http.StatusOK
			}
			writeJSON(writer, status, newLibraryScanResponse(scans.Status()))
		})
	}
	if settings != nil {
		mux.HandleFunc("GET /api/v1/settings/transfers", func(writer http.ResponseWriter, request *http.Request) {
			actor, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage)
			if !ok {
				return
			}
			limits, err := settings.Limits(request.Context(), actor)
			if err != nil {
				writeTransferSettingsError(writer, err)
				return
			}
			writeJSON(writer, http.StatusOK, newTransferSettingsResponse(limits))
		})
		mux.HandleFunc("PUT /api/v1/settings/transfers", func(writer http.ResponseWriter, request *http.Request) {
			actor, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage)
			if !ok {
				return
			}
			request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
			decoder := json.NewDecoder(request.Body)
			decoder.DisallowUnknownFields()
			var payload transferSettingsResponse
			if err := decoder.Decode(&payload); err != nil {
				http.Error(writer, "invalid transfer settings", http.StatusBadRequest)
				return
			}
			if err := decoder.Decode(&struct{}{}); err != io.EOF {
				http.Error(writer, "invalid transfer settings", http.StatusBadRequest)
				return
			}
			limits, err := settings.UpdateLimits(request.Context(), actor, domain.TransferLimits{
				UploadBytesPerSecond:   payload.UploadLimitBytesPerSecond,
				DownloadBytesPerSecond: payload.DownloadLimitBytesPerSecond,
			})
			if err != nil {
				writeTransferSettingsError(writer, err)
				return
			}
			writeJSON(writer, http.StatusOK, newTransferSettingsResponse(limits))
		})
	}
	mux.HandleFunc("GET /api/v1/providers/soulseek/status", func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSoulseekSearch); !ok {
			return
		}
		status := domain.ProviderStatus{
			Provider: "soulseek", Message: "slskd is not configured",
		}
		if providerStatus != nil {
			status = providerStatus.ProviderStatus(request.Context())
		}
		writeJSON(writer, http.StatusOK, providerStatusResponse{
			Provider: status.Provider, Configured: status.Configured, Reachable: status.Reachable,
			Authenticated: status.Authenticated, Message: status.Message,
		})
	})
	mux.HandleFunc("POST /api/v1/providers/soulseek/search", func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSoulseekSearch); !ok {
			return
		}
		if providerSearch == nil {
			http.Error(writer, "Soulseek provider is not configured", http.StatusServiceUnavailable)
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		var payload sourceSearchRequest
		if err := decoder.Decode(&payload); err != nil {
			http.Error(writer, "invalid search request", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(writer, "invalid search request", http.StatusBadRequest)
			return
		}
		payload.Query = strings.TrimSpace(payload.Query)
		if length := len([]rune(payload.Query)); length < 3 || length > 200 || payload.Limit < 0 || payload.Limit > 200 {
			http.Error(writer, "query must contain 3-200 characters and limit must not exceed 200", http.StatusBadRequest)
			return
		}
		if payload.Limit == 0 {
			payload.Limit = 50
		}
		results, err := providerSearch.Search(request.Context(), domain.SearchQuery{
			Text: payload.Query, Limit: payload.Limit,
		})
		if err != nil {
			if errors.Is(err, ports.ErrSourceUnavailable) {
				http.Error(writer, "Soulseek provider is unavailable", http.StatusServiceUnavailable)
				return
			}
			http.Error(writer, "search Soulseek provider", http.StatusBadGateway)
			return
		}
		response := sourceSearchResponse{
			Query: payload.Query, Results: make([]sourceSearchResultResponse, 0, len(results)),
		}
		for _, result := range results {
			response.Results = append(response.Results, sourceSearchResultResponse{
				ID: result.Track.ID, Title: result.Track.Title, Artist: result.Track.Artist,
				Album: result.Track.Album, Path: result.DisplayPath,
				Size: result.Track.Size, DurationSeconds: int64(result.Track.Duration / time.Second),
				BitRate: result.Track.BitRate, Suffix: result.Track.Suffix,
				Peer: result.Availability.Peer, UploadSpeedBytesPerSecond: result.Availability.UploadSpeed,
				QueueLength: result.Availability.QueueLength, FreeUploadSlot: result.Availability.FreeUploadSlot,
				RequiresApproval: result.Availability.RequiresApproval,
			})
		}
		response.Collections = groupSourceSearchResults(response.Results)
		writeJSON(writer, http.StatusOK, response)
	})
	mux.HandleFunc("POST /api/v1/providers/soulseek/tracks/{id}", func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSoulseekAdd); !ok {
			return
		}
		adder, ok := providerSearch.(sourceAdder)
		if !ok {
			http.Error(writer, "Soulseek provider is not configured", http.StatusServiceUnavailable)
			return
		}
		id := strings.TrimSpace(request.PathValue("id"))
		if id == "" {
			http.Error(writer, "track ID is required", http.StatusBadRequest)
			return
		}
		track, err := adder.Add(request.Context(), id)
		if err != nil {
			if errors.Is(err, services.ErrDiscoveryResultNotFound) {
				http.Error(writer, "search result expired; search again", http.StatusNotFound)
				return
			}
			http.Error(writer, "add Soulseek track to library", http.StatusInternalServerError)
			return
		}
		writeJSON(writer, http.StatusCreated, sourceAddResponse{
			ID: track.ID, Title: track.Title, Artist: track.Artist, Album: track.Album,
		})
	})
	mux.HandleFunc("GET /api/v1/providers/soulseek/albums/{id}", func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSoulseekSearch); !ok {
			return
		}
		browser, ok := providerSearch.(sourceCollectionAdder)
		if !ok {
			http.Error(writer, "Soulseek album browsing is not configured", http.StatusServiceUnavailable)
			return
		}
		id := strings.TrimSpace(request.PathValue("id"))
		if id == "" {
			http.Error(writer, "track ID is required", http.StatusBadRequest)
			return
		}
		collection, err := browser.PreviewCollection(request.Context(), id)
		if err != nil {
			writeSourceCollectionError(writer, err, "preview Soulseek album")
			return
		}
		response := sourceCollectionPreviewResponse{
			Name: collection.Name, Artist: collection.Artist, HasArtwork: collection.CoverArtID != "",
			Tracks: make([]sourceCollectionPreviewTrackResponse, 0, len(collection.Tracks)),
		}
		for _, source := range collection.Tracks {
			response.Tracks = append(response.Tracks, sourceCollectionPreviewTrackResponse{
				ID: source.Track.ID, Title: source.Track.Title, Size: source.Track.Size,
				DurationSeconds: int64(source.Track.Duration / time.Second), Suffix: source.Track.Suffix,
			})
		}
		writeJSON(writer, http.StatusOK, response)
	})
	mux.HandleFunc("POST /api/v1/providers/soulseek/albums/{id}", func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSoulseekAdd); !ok {
			return
		}
		adder, ok := providerSearch.(sourceCollectionAdder)
		if !ok {
			http.Error(writer, "Soulseek album browsing is not configured", http.StatusServiceUnavailable)
			return
		}
		id := strings.TrimSpace(request.PathValue("id"))
		if id == "" {
			http.Error(writer, "track ID is required", http.StatusBadRequest)
			return
		}
		collection, err := adder.AddCollection(request.Context(), id)
		if err != nil {
			writeSourceCollectionError(writer, err, "add Soulseek album to library")
			return
		}
		writeJSON(writer, http.StatusCreated, sourceCollectionAddResponse{
			Name: collection.Name, Artist: collection.Artist, Tracks: len(collection.Tracks),
		})
	})
	if cache != nil {
		mux.HandleFunc("GET /api/v1/cache/status", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionMonitoringView); !ok {
				return
			}
			stats, err := cache.Stats(request.Context())
			if err != nil {
				http.Error(writer, "read media cache status", http.StatusInternalServerError)
				return
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			response := cacheStatusResponse{
				CapacityBytes: stats.Capacity, SizeBytes: stats.Size, Entries: stats.Entries,
				PinnedSizeBytes: stats.PinnedSize, PinnedEntries: stats.PinnedEntries,
				Components: make([]cacheComponentResponse, 0, len(stats.Components)),
			}
			for _, component := range stats.Components {
				response.Components = append(response.Components, cacheComponentResponse{
					Name: component.Name, SizeBytes: component.Size, Entries: component.Entries,
					PartialEntries: component.PartialEntries,
				})
			}
			_ = json.NewEncoder(writer).Encode(response)
		})
	}
	if torrentImporter != nil {
		mux.HandleFunc("POST /api/v1/torrents", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
				return
			}
			request.Body = http.MaxBytesReader(writer, request.Body, 16<<20)
			result, err := torrentImporter.Import(request.Context(), request.Body)
			if errors.Is(err, scanner.ErrScanInProgress) {
				http.Error(writer, "library scan is already in progress", http.StatusConflict)
				return
			}
			if err != nil {
				http.Error(writer, "invalid torrent metadata", http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Tracks int    `json:"tracks"`
			}{ID: result.SourceID, Name: result.Name, Tracks: result.Tracks})
		})
	}
	if uriImporter != nil {
		mux.HandleFunc("POST /api/v1/torrents/magnet", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
				return
			}
			request.Body = http.MaxBytesReader(writer, request.Body, 64<<10)
			decoder := json.NewDecoder(request.Body)
			decoder.DisallowUnknownFields()
			var payload struct {
				Magnet string `json:"magnet"`
			}
			if err := decoder.Decode(&payload); err != nil || strings.TrimSpace(payload.Magnet) == "" {
				http.Error(writer, "invalid magnet request", http.StatusBadRequest)
				return
			}
			if err := decoder.Decode(&struct{}{}); err != io.EOF {
				http.Error(writer, "invalid magnet request", http.StatusBadRequest)
				return
			}
			item, err := uriImporter.ImportURI(request.Context(), payload.Magnet)
			if err != nil {
				http.Error(writer, "invalid magnet URI", http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			writer.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(writer).Encode(newSourceImportResponse(item))
		})
		mux.HandleFunc("GET /api/v1/imports", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
				return
			}
			items, err := uriImporter.SourceImports(request.Context())
			if err != nil {
				http.Error(writer, "read source imports", http.StatusInternalServerError)
				return
			}
			response := sourceImportsResponse{Imports: make([]sourceImportResponse, 0, len(items))}
			for _, item := range items {
				response.Imports = append(response.Imports, newSourceImportResponse(item))
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(writer).Encode(response)
		})
	}
	if sources != nil {
		mux.HandleFunc("GET /api/v1/torrents", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
				return
			}
			items, err := sources.ManagedSources(request.Context())
			if err != nil {
				http.Error(writer, "read torrent sources", http.StatusInternalServerError)
				return
			}
			response := managedSourcesResponse{Sources: make([]managedSourceResponse, 0, len(items))}
			for _, item := range items {
				response.Sources = append(response.Sources, managedSourceResponse{
					Provider: item.Provider, ID: item.ID, Name: item.Name, Tracks: item.Tracks,
					Attached: item.Attached, Paused: item.Paused, Pinned: item.Pinned,
				})
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(writer).Encode(response)
		})
		for action, update := range map[string]func(context.Context, string) error{
			"pause":  sources.PauseSource,
			"resume": sources.ResumeSource,
			"pin": func(ctx context.Context, id string) error {
				return sources.PinSource(ctx, id, true)
			},
			"unpin": func(ctx context.Context, id string) error {
				return sources.PinSource(ctx, id, false)
			},
		} {
			mux.HandleFunc("POST /api/v1/torrents/{id}/"+action, func(writer http.ResponseWriter, request *http.Request) {
				if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
					return
				}
				if err := update(request.Context(), request.PathValue("id")); err != nil {
					writeSourceManagementError(writer, err)
					return
				}
				writer.WriteHeader(http.StatusNoContent)
			})
		}
		mux.HandleFunc("DELETE /api/v1/torrents/{id}", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
				return
			}
			deleteData := false
			if value := request.URL.Query().Get("deleteData"); value != "" {
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					http.Error(writer, "deleteData must be a boolean", http.StatusBadRequest)
					return
				}
				deleteData = parsed
			}
			if err := sources.RemoveSource(request.Context(), request.PathValue("id"), deleteData); err != nil {
				writeSourceManagementError(writer, err)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		})
	}
	if transfers != nil {
		mux.HandleFunc("GET /api/v1/transfers", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionMonitoringView); !ok {
				return
			}
			items, err := transfers.Transfers(request.Context())
			if err != nil {
				http.Error(writer, "read source transfer status", http.StatusInternalServerError)
				return
			}
			response := transfersResponse{Transfers: make([]transferResponse, 0, len(items))}
			for _, item := range items {
				response.Transfers = append(response.Transfers, transferResponse{
					Provider: item.Provider, ID: item.ID, Name: item.Name,
					CompletedBytes: item.CompletedBytes, TotalBytes: item.TotalBytes,
					DownloadedBytes: item.DownloadedBytes, UploadedBytes: item.UploadedBytes,
					DownloadLimit: item.DownloadLimit, UploadLimit: item.UploadLimit,
					Peers: item.Peers, ActivePeers: item.ActivePeers,
					ConnectedSeeders: item.ConnectedSeeders,
					ActiveStreams:    item.ActiveStreams,
					Seeding:          item.Seeding,
				})
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(writer).Encode(response)
		})
	}
	if downloads != nil {
		mux.HandleFunc("GET /api/v1/downloads", func(writer http.ResponseWriter, request *http.Request) {
			if _, ok := requirePermission(writer, request, authenticator, domain.PermissionMonitoringView); !ok {
				return
			}
			items, err := downloads.TrackDownloads(request.Context())
			if err != nil {
				http.Error(writer, "read track download status", http.StatusInternalServerError)
				return
			}
			response := trackDownloadsResponse{Downloads: make([]trackDownloadResponse, 0, len(items))}
			for _, item := range items {
				response.Downloads = append(response.Downloads, trackDownloadResponse{
					ID: item.ID, Provider: item.Provider, SourceID: item.SourceID, TrackID: item.TrackID,
					Name: item.Name, State: item.State, CompletedBytes: item.CompletedBytes,
					TotalBytes: item.TotalBytes, Error: item.Error,
					StartedAt: item.StartedAt.Format(time.RFC3339), UpdatedAt: item.UpdatedAt.Format(time.RFC3339),
				})
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(writer).Encode(response)
		})
		if controller, ok := downloads.(ports.TrackDownloadController); ok {
			mux.HandleFunc("DELETE /api/v1/downloads/{id}", func(writer http.ResponseWriter, request *http.Request) {
				if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
					return
				}
				if err := controller.CancelTrackDownload(request.Context(), request.PathValue("id")); err != nil {
					writeDownloadActionError(writer, err)
					return
				}
				writer.WriteHeader(http.StatusNoContent)
			})
			mux.HandleFunc("POST /api/v1/downloads/{id}/retry", func(writer http.ResponseWriter, request *http.Request) {
				if _, ok := requirePermission(writer, request, authenticator, domain.PermissionSourcesManage); !ok {
					return
				}
				if err := controller.RetryTrackDownload(request.Context(), request.PathValue("id")); err != nil {
					writeDownloadActionError(writer, err)
					return
				}
				writer.WriteHeader(http.StatusNoContent)
			})
		}
	}
	if users != nil {
		registerUserRoutes(mux, authenticator, users)
	}
	return mux
}

func writeDownloadActionError(writer http.ResponseWriter, err error) {
	if errors.Is(err, ports.ErrNotFound) {
		http.Error(writer, "track download not found", http.StatusNotFound)
		return
	}
	http.Error(writer, "change track download", http.StatusConflict)
}

func writeSourceCollectionError(writer http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, services.ErrDiscoveryResultNotFound):
		http.Error(writer, "search result expired; search again", http.StatusNotFound)
	case errors.Is(err, ports.ErrSourceUnavailable):
		http.Error(writer, "Soulseek peer is unavailable", http.StatusBadGateway)
	default:
		http.Error(writer, action, http.StatusBadGateway)
	}
}

func groupSourceSearchResults(results []sourceSearchResultResponse) []sourceSearchCollectionResponse {
	groups := make([]sourceSearchCollectionResponse, 0, len(results))
	indexes := make(map[string]int, len(results))
	for _, result := range results {
		directory := sourceResultDirectory(result.Path)
		key := result.Peer + "\x00" + directory + "\x00" + strconv.FormatBool(result.RequiresApproval)
		index, ok := indexes[key]
		if !ok {
			name := strings.TrimSpace(result.Album)
			if name == "" {
				name = sourceResultDirectoryName(directory)
			}
			artist := strings.TrimSpace(result.Artist)
			if artist == "" {
				artist = "Unknown Artist"
			}
			groups = append(groups, sourceSearchCollectionResponse{
				ID: result.ID, Name: name, Artist: artist, Path: directory, Peer: result.Peer,
				UploadSpeedBytesPerSecond: result.UploadSpeedBytesPerSecond,
				QueueLength:               result.QueueLength, FreeUploadSlot: result.FreeUploadSlot,
				RequiresApproval: result.RequiresApproval,
				Results:          make([]sourceSearchResultResponse, 0, 1),
			})
			index = len(groups) - 1
			indexes[key] = index
		}
		group := &groups[index]
		group.Results = append(group.Results, result)
		group.MatchedTracks++
		group.MatchedSize += result.Size
		if result.UploadSpeedBytesPerSecond > group.UploadSpeedBytesPerSecond {
			group.UploadSpeedBytesPerSecond = result.UploadSpeedBytesPerSecond
		}
		if result.FreeUploadSlot {
			group.FreeUploadSlot = true
		}
		if result.QueueLength < group.QueueLength {
			group.QueueLength = result.QueueLength
		}
		format := strings.ToUpper(strings.TrimSpace(result.Suffix))
		if format != "" && !stringSliceContains(group.Formats, format) {
			group.Formats = append(group.Formats, format)
		}
	}
	return groups
}

func sourceResultDirectory(path string) string {
	path = strings.Trim(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/"), "/")
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[:index]
	}
	return ""
}

func sourceResultDirectoryName(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		path = path[index+1:]
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "Unknown Album"
	}
	return path
}

func stringSliceContains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func newLibraryScanResponse(status scanner.Status) libraryScanResponse {
	response := libraryScanResponse{
		Scanning: status.Scanning, Tracks: status.Count, LastError: status.LastError,
	}
	if !status.LastStartedAt.IsZero() {
		response.LastStartedAt = status.LastStartedAt.Format(time.RFC3339)
	}
	if !status.LastFinishedAt.IsZero() {
		response.LastFinishedAt = status.LastFinishedAt.Format(time.RFC3339)
	}
	return response
}

func newTransferSettingsResponse(limits domain.TransferLimits) transferSettingsResponse {
	return transferSettingsResponse{
		UploadLimitBytesPerSecond:   limits.UploadBytesPerSecond,
		DownloadLimitBytesPerSecond: limits.DownloadBytesPerSecond,
	}
}

func writeTransferSettingsError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidSettings):
		http.Error(writer, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ports.ErrForbidden):
		http.Error(writer, "operation is not permitted", http.StatusForbidden)
	default:
		http.Error(writer, "manage transfer settings", http.StatusInternalServerError)
	}
}

func decodeJSONRequest(writer http.ResponseWriter, request *http.Request, limit int64, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, limit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(writer, "invalid JSON request", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(writer, "invalid JSON request", http.StatusBadRequest)
		return false
	}
	return true
}

func writeCatalogManagementError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		http.Error(writer, "catalog item not found", http.StatusNotFound)
	case strings.Contains(err.Error(), "different"), strings.Contains(err.Error(), "cycle"),
		strings.Contains(err.Error(), "cannot be"):
		http.Error(writer, err.Error(), http.StatusBadRequest)
	default:
		http.Error(writer, "manage catalog", http.StatusInternalServerError)
	}
}

func newSourceImportResponse(item domain.SourceImport) sourceImportResponse {
	return sourceImportResponse{
		ID: item.ID, Provider: item.Provider, Name: item.Name, SourceID: item.SourceID,
		Tracks: item.Tracks, State: item.State, Error: item.Error,
		CreatedAt: item.CreatedAt.Format(time.RFC3339), UpdatedAt: item.UpdatedAt.Format(time.RFC3339),
	}
}

type fixedAuthenticator struct {
	username string
	password string
}

func (a fixedAuthenticator) AuthenticatePassword(
	_ context.Context, username, password string,
) (domain.User, error) {
	if subtle.ConstantTimeCompare([]byte(username), []byte(a.username)) != 1 ||
		subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) != 1 {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	return domain.User{Username: a.username, Role: domain.UserRoleAdmin}, nil
}

func (fixedAuthenticator) AuthenticateToken(
	context.Context, string, string, string,
) (domain.User, error) {
	return domain.User{}, ports.ErrAuthenticationFailed
}

func authenticateBasic(request *http.Request, authenticator ports.Authenticator) (domain.User, error) {
	providedUsername, providedPassword, ok := request.BasicAuth()
	if !ok || authenticator == nil {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	return authenticator.AuthenticatePassword(request.Context(), providedUsername, providedPassword)
}

func requireAdministrator(
	writer http.ResponseWriter, request *http.Request, authenticator ports.Authenticator,
) (domain.User, bool) {
	user, err := authenticateBasic(request, authenticator)
	if err != nil {
		writer.Header().Set("WWW-Authenticate", `Basic realm="Peerphonic"`)
		http.Error(writer, "authentication required", http.StatusUnauthorized)
		return domain.User{}, false
	}
	if !user.IsAdmin() {
		http.Error(writer, "administrator access required", http.StatusForbidden)
		return domain.User{}, false
	}
	return user, true
}

func requirePermission(
	writer http.ResponseWriter,
	request *http.Request,
	authenticator ports.Authenticator,
	permission domain.Permission,
) (domain.User, bool) {
	user, err := authenticateBasic(request, authenticator)
	if err != nil {
		writer.Header().Set("WWW-Authenticate", `Basic realm="Peerphonic"`)
		http.Error(writer, "authentication required", http.StatusUnauthorized)
		return domain.User{}, false
	}
	if !user.HasPermission(permission) {
		http.Error(writer, "permission required: "+string(permission), http.StatusForbidden)
		return domain.User{}, false
	}
	return user, true
}

func writeSourceManagementError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		http.Error(writer, "torrent source not found", http.StatusNotFound)
	case errors.Is(err, ports.ErrSourceBusy), errors.Is(err, scanner.ErrScanInProgress):
		http.Error(writer, err.Error(), http.StatusConflict)
	default:
		http.Error(writer, "manage torrent source", http.StatusInternalServerError)
	}
}
