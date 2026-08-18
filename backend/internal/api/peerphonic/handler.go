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
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type cacheStatus interface {
	Stats(ctx context.Context) (domain.CacheStats, error)
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

func NewHandler(
	cache cacheStatus,
	torrentImporter ports.SourceImporter,
	uriImporter ports.SourceURIImporter,
	sources ports.SourceManager,
	transfers ports.SourceTransferMonitor,
	downloads ports.TrackDownloadMonitor,
	username, password string,
) http.Handler {
	return newHandler(
		cache, torrentImporter, uriImporter, sources, transfers, downloads,
		fixedAuthenticator{username: username, password: password}, nil,
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
) http.Handler {
	return newHandler(cache, torrentImporter, uriImporter, sources, transfers, downloads, authenticator, users)
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
	}
	if users != nil {
		registerUserRoutes(mux, authenticator, users)
	}
	return mux
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
