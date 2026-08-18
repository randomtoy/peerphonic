package peerphonic

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"

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
	Peers            int    `json:"peers"`
	ActivePeers      int    `json:"activePeers"`
	ConnectedSeeders int    `json:"connectedSeeders"`
	ActiveStreams    int    `json:"activeStreams"`
	Seeding          bool   `json:"seeding"`
}

type transfersResponse struct {
	Transfers []transferResponse `json:"transfers"`
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
	transfers ports.SourceTransferMonitor,
	username, password string,
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
			if !basicAuthenticated(request, username, password) {
				writer.Header().Set("WWW-Authenticate", `Basic realm="Peerphonic"`)
				http.Error(writer, "authentication required", http.StatusUnauthorized)
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
	if transfers != nil {
		mux.HandleFunc("GET /api/v1/transfers", func(writer http.ResponseWriter, request *http.Request) {
			if !basicAuthenticated(request, username, password) {
				writer.Header().Set("WWW-Authenticate", `Basic realm="Peerphonic"`)
				http.Error(writer, "authentication required", http.StatusUnauthorized)
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
	return mux
}

func basicAuthenticated(request *http.Request, username, password string) bool {
	providedUsername, providedPassword, ok := request.BasicAuth()
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(providedUsername), []byte(username)) == 1 &&
		subtle.ConstantTimeCompare([]byte(providedPassword), []byte(password)) == 1
}
