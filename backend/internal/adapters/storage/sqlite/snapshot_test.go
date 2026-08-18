package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotterCreatesConsistentSnapshotWhileCatalogIsOpen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "peerphonic.db")
	catalog, err := Open(ctx, sourcePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer catalog.Close()
	if _, err := catalog.db.ExecContext(ctx, "CREATE TABLE backup_marker (value TEXT NOT NULL)"); err != nil {
		t.Fatalf("create marker: %v", err)
	}
	if _, err := catalog.db.ExecContext(ctx, "INSERT INTO backup_marker(value) VALUES ('present')"); err != nil {
		t.Fatalf("insert marker: %v", err)
	}

	snapshotter, err := NewSnapshotter(sourcePath)
	if err != nil {
		t.Fatalf("NewSnapshotter() error = %v", err)
	}
	destination := filepath.Join(root, "snapshot.db")
	if err := snapshotter.Snapshot(ctx, destination); err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	snapshot, err := sql.Open("sqlite", readOnlySQLiteDSN(destination))
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snapshot.Close()
	var marker string
	if err := snapshot.QueryRowContext(ctx, "SELECT value FROM backup_marker").Scan(&marker); err != nil {
		t.Fatalf("query snapshot marker: %v", err)
	}
	if marker != "present" {
		t.Fatalf("marker = %q", marker)
	}
}

func TestSnapshotterRefusesExistingDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "peerphonic.db")
	catalog, err := Open(ctx, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	snapshotter, err := NewSnapshotter(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "existing.db")
	if err := os.WriteFile(destination, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := snapshotter.Snapshot(ctx, destination); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Snapshot() error = %v", err)
	}
}
