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

func NewHandler(cache cacheStatus, torrentImporter ports.SourceImporter, username, password string) http.Handler {
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
			_ = json.NewEncoder(writer).Encode(struct {
				CapacityBytes   int64 `json:"capacityBytes"`
				SizeBytes       int64 `json:"sizeBytes"`
				Entries         int   `json:"entries"`
				PinnedSizeBytes int64 `json:"pinnedSizeBytes"`
				PinnedEntries   int   `json:"pinnedEntries"`
			}{
				CapacityBytes: stats.Capacity, SizeBytes: stats.Size, Entries: stats.Entries,
				PinnedSizeBytes: stats.PinnedSize, PinnedEntries: stats.PinnedEntries,
			})
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
