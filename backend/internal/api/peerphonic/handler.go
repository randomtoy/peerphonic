package peerphonic

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type cacheStatus interface {
	Stats(ctx context.Context) (domain.CacheStats, error)
}

func NewHandler(caches ...cacheStatus) http.Handler {
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
	if len(caches) > 0 && caches[0] != nil {
		mux.HandleFunc("GET /api/v1/cache/status", func(writer http.ResponseWriter, request *http.Request) {
			stats, err := caches[0].Stats(request.Context())
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
	return mux
}
