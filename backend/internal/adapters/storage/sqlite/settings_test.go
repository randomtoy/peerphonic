package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestTransferLimitsPersist(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "peerphonic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	if _, err := catalog.TransferLimits(ctx); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("initial TransferLimits() error = %v", err)
	}
	want := domain.TransferLimits{UploadBytesPerSecond: 1024, DownloadBytesPerSecond: 2048}
	if err := catalog.SaveTransferLimits(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := catalog.TransferLimits(ctx)
	if err != nil || got != want {
		t.Fatalf("TransferLimits() = %#v, %v", got, err)
	}
}
