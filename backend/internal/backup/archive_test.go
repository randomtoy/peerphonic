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
