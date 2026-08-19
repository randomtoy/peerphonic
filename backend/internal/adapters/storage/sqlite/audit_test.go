package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func TestAuditEntriesPersistNewestFirst(t *testing.T) {
	t.Parallel()

	catalog, err := Open(context.Background(), filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	for index, path := range []string{"/api/v1/first", "/api/v1/second"} {
		if err := catalog.AppendAuditEntry(context.Background(), domain.AuditEntry{
			OccurredAt: time.Date(2026, 8, 19, 12, index, 0, 0, time.UTC), RequestID: path,
			Actor: "admin", RemoteAddress: "127.0.0.1", Method: "POST", Path: path, Status: 204,
		}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := catalog.AuditEntries(context.Background(), 1)
	if err != nil || len(entries) != 1 || entries[0].Path != "/api/v1/second" {
		t.Fatalf("AuditEntries() = %#v, %v", entries, err)
	}
}
