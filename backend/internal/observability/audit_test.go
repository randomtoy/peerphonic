package observability

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type auditStoreStub struct{ entries []domain.AuditEntry }

func (s *auditStoreStub) AppendAuditEntry(_ context.Context, entry domain.AuditEntry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func (s *auditStoreStub) AuditEntries(context.Context, int) ([]domain.AuditEntry, error) {
	return s.entries, nil
}

func TestAuditMiddlewarePersistsMutationsWithoutSecrets(t *testing.T) {
	t.Parallel()

	store := &auditStoreStub{}
	middleware := NewAuditMiddleware(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	middleware.now = func() time.Time { return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC) }
	handler := middleware.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/users?password=secret", nil)
	request.RemoteAddr = "192.0.2.4:9876"
	request.SetBasicAuth("Admin", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if len(store.entries) != 1 {
		t.Fatalf("audit entries = %#v", store.entries)
	}
	entry := store.entries[0]
	if entry.Actor != "Admin" || entry.Path != "/api/v1/users" || entry.Status != http.StatusCreated || entry.RemoteAddress != "192.0.2.4" {
		t.Fatalf("audit entry = %#v", entry)
	}
	if response.Header().Get("X-Request-ID") == "" || entry.RequestID != response.Header().Get("X-Request-ID") {
		t.Fatalf("request ID entry = %q, header = %q", entry.RequestID, response.Header().Get("X-Request-ID"))
	}

	read := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	handler.ServeHTTP(httptest.NewRecorder(), read)
	if len(store.entries) != 1 {
		t.Fatalf("read request was audited: %#v", store.entries)
	}
}
