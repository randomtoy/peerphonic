package peerphonic

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func NewAuditHandler(store ports.AuditStore, authenticator ports.Authenticator) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := requirePermission(writer, request, authenticator, domain.PermissionMonitoringView); !ok {
			return
		}
		limit := 100
		if raw := request.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 500 {
				http.Error(writer, "limit must be between 1 and 500", http.StatusBadRequest)
				return
			}
			limit = parsed
		}
		entries, err := store.AuditEntries(request.Context(), limit)
		if err != nil {
			http.Error(writer, "list audit entries", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(writer).Encode(map[string]any{"entries": entries})
	})
}
