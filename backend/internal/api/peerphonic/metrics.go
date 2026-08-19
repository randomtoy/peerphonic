package peerphonic

import (
	"net/http"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func NewMetricsHandler(metrics http.Handler, authenticator ports.Authenticator) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := requirePermission(
			writer, request, authenticator, domain.PermissionMonitoringView,
		); !ok {
			return
		}
		metrics.ServeHTTP(writer, request)
	})
}
