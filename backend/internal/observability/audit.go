package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type AuditMiddleware struct {
	store  ports.AuditStore
	logger *slog.Logger
	now    func() time.Time
}

func NewAuditMiddleware(store ports.AuditStore, logger *slog.Logger) *AuditMiddleware {
	return &AuditMiddleware{store: store, logger: logger, now: time.Now}
}

func (m *AuditMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !isAuditedMutation(request) {
			next.ServeHTTP(writer, request)
			return
		}
		requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = randomRequestID()
		}
		writer.Header().Set("X-Request-ID", requestID)
		observed := &responseObserver{ResponseWriter: writer}
		next.ServeHTTP(observed, request)
		status := observed.status
		if status == 0 {
			status = http.StatusOK
		}
		actor, _, ok := request.BasicAuth()
		if !ok || strings.TrimSpace(actor) == "" {
			actor = "anonymous"
		}
		remote := request.RemoteAddr
		if host, _, err := net.SplitHostPort(remote); err == nil {
			remote = host
		}
		entry := domain.AuditEntry{
			OccurredAt: m.now().UTC(), RequestID: requestID, Actor: actor,
			RemoteAddress: remote, Method: request.Method, Path: request.URL.Path, Status: status,
		}
		if err := m.store.AppendAuditEntry(context.WithoutCancel(request.Context()), entry); err != nil {
			m.logger.Error("record administrative audit entry", "error", err, "request_id", requestID)
		}
		m.logger.Info("administrative request", "request_id", requestID, "actor", actor,
			"remote", remote, "method", request.Method, "path", request.URL.Path, "status", status)
	})
}

func isAuditedMutation(request *http.Request) bool {
	if request.URL.Path != "/api/v1" && !strings.HasPrefix(request.URL.Path, "/api/v1/") {
		return false
	}
	switch request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func randomRequestID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(buffer)
}
