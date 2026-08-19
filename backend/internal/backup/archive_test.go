package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type snapshotterStub struct {
	contents []byte
	calls    int
}

func (s *snapshotterStub) Snapshot(_ context.Context, destination string) error {
	s.calls++
	return os.WriteFile(destination, s.contents, 0o600)
}

func TestCreateWritesProtectedVerifiedArchive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	key := bytes.Repeat([]byte{0x42}, 32)
	keyPath := filepath.Join(root, "peerphonic.db.auth.key")
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	database := []byte("sqlite snapshot")
	snapshotter := &snapshotterStub{contents: database}
	output := filepath.Join(root, "backups", "peerphonic.tar.gz")
	manifest, err := Create(context.Background(), snapshotter, Options{
		OutputPath: output, CredentialKeyPath: keyPath,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if snapshotter.calls != 1 || manifest.FormatVersion != FormatVersion || len(manifest.Files) != 2 {
		t.Fatalf("calls = %d, manifest = %#v", snapshotter.calls, manifest)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode = %o", info.Mode().Perm())
	}

	entries := readArchive(t, output)
	for name, expected := range map[string][]byte{
		"peerphonic.db": database, "peerphonic.db.auth.key": key,
	} {
		entry, ok := entries[name]
		if !ok || !bytes.Equal(entry.contents, expected) || entry.mode != 0o600 {
			t.Fatalf("archive entry %q = %#v", name, entry)
		}
	}
	var archivedManifest Manifest
	if err := json.Unmarshal(entries["manifest.json"].contents, &archivedManifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if archivedManifest.FormatVersion != FormatVersion || len(archivedManifest.Files) != 2 {
		t.Fatalf("archived manifest = %#v", archivedManifest)
	}
	for _, file := range archivedManifest.Files {
		entry := entries[file.Name]
		digest := sha256.Sum256(entry.contents)
		if file.Size != int64(len(entry.contents)) || file.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("manifest file = %#v", file)
		}
	}
}

func TestCreateDoesNotOverwriteExistingArchive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	output := filepath.Join(root, "backup.tar.gz")
	if err := os.WriteFile(output, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(root, "auth.key")
	if err := os.WriteFile(keyPath, make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshotter := &snapshotterStub{contents: []byte("database")}
	_, err := Create(context.Background(), snapshotter, Options{
		OutputPath: output, CredentialKeyPath: keyPath,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") || snapshotter.calls != 0 {
		t.Fatalf("Create() error = %v, calls = %d", err, snapshotter.calls)
	}
	contents, err := os.ReadFile(output)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("output = %q, error = %v", contents, err)
	}
}

func TestRestoreValidatesAndRestoresBothFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	key := bytes.Repeat([]byte{0x24}, 32)
	keyPath := filepath.Join(root, "source.auth.key")
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "backup.tar.gz")
	_, err := Create(context.Background(), &snapshotterStub{contents: []byte("database snapshot")}, Options{
		OutputPath: archivePath, CredentialKeyPath: keyPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	databaseTarget := filepath.Join(root, "restored", "peerphonic.db")
	credentialTarget := databaseTarget + ".auth.key"
	result, err := Restore(context.Background(), RestoreOptions{
		InputPath: archivePath, DatabasePath: databaseTarget, CredentialKeyPath: credentialTarget,
	})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if result.Manifest.FormatVersion != FormatVersion || len(result.Replaced) != 0 {
		t.Fatalf("Restore() result = %#v", result)
	}
	for path, expected := range map[string][]byte{
		databaseTarget: []byte("database snapshot"), credentialTarget: key,
	} {
		contents, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(contents, expected) {
			t.Fatalf("restored %q = %q, error = %v", path, contents, err)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("restored %q mode = %o", path, info.Mode().Perm())
		}
	}
}

func TestRestoreRequiresForceAndPreservesRecoveryCopies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	keyPath := filepath.Join(root, "source.auth.key")
	if err := os.WriteFile(keyPath, make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "backup.tar.gz")
	if _, err := Create(context.Background(), &snapshotterStub{contents: []byte("new database")}, Options{
		OutputPath: archivePath, CredentialKeyPath: keyPath,
	}); err != nil {
		t.Fatal(err)
	}
	databaseTarget := filepath.Join(root, "peerphonic.db")
	credentialTarget := databaseTarget + ".auth.key"
	if err := os.WriteFile(databaseTarget, []byte("old database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialTarget, bytes.Repeat([]byte{1}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	options := RestoreOptions{InputPath: archivePath, DatabasePath: databaseTarget, CredentialKeyPath: credentialTarget}
	if _, err := Restore(context.Background(), options); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("Restore() error = %v", err)
	}
	options.Force = true
	result, err := Restore(context.Background(), options)
	if err != nil {
		t.Fatalf("Restore(force) error = %v", err)
	}
	if len(result.Replaced) != 2 {
		t.Fatalf("recovery copies = %v", result.Replaced)
	}
	for _, path := range result.Replaced {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("recovery copy %q: %v", path, err)
		}
	}
}

func TestRestoreRejectsChecksumMismatchWithoutTouchingTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	archivePath := filepath.Join(root, "corrupt.tar.gz")
	writeTestArchive(t, archivePath, map[string][]byte{
		"manifest.json": []byte(`{"formatVersion":1,"createdAt":"2026-01-01T00:00:00Z","files":[{"name":"peerphonic.db","size":3,"sha256":"bad"},{"name":"peerphonic.db.auth.key","size":32,"sha256":"bad"}]}`),
		"peerphonic.db": []byte("db!"), "peerphonic.db.auth.key": make([]byte, 32),
	})
	target := filepath.Join(root, "target.db")
	if _, err := Restore(context.Background(), RestoreOptions{
		InputPath: archivePath, DatabasePath: target, CredentialKeyPath: target + ".auth.key",
	}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target unexpectedly exists: %v", err)
	}
}

func writeTestArchive(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	for _, name := range []string{"manifest.json", "peerphonic.db", "peerphonic.db.auth.key"} {
		contents := entries[name]
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

type archiveEntry struct {
	contents []byte
	mode     int64
}

func readArchive(t *testing.T, path string) map[string]archiveEntry {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	entries := make(map[string]archiveEntry)
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = archiveEntry{contents: contents, mode: header.Mode}
	}
	return entries
}
