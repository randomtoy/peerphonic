package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const FormatVersion = 1

type Snapshotter interface {
	Snapshot(ctx context.Context, destination string) error
}

type Options struct {
	OutputPath        string
	CredentialKeyPath string
}

type Manifest struct {
	FormatVersion int            `json:"formatVersion"`
	CreatedAt     string         `json:"createdAt"`
	Files         []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func Create(ctx context.Context, snapshotter Snapshotter, options Options) (Manifest, error) {
	if snapshotter == nil {
		return Manifest{}, fmt.Errorf("metadata snapshotter is required")
	}
	if options.OutputPath == "" {
		return Manifest{}, fmt.Errorf("backup output path is required")
	}
	if options.CredentialKeyPath == "" {
		return Manifest{}, fmt.Errorf("credential key path is required")
	}
	if err := ensureMissing(options.OutputPath); err != nil {
		return Manifest{}, err
	}
	if err := ensureCredentialKey(options.CredentialKeyPath); err != nil {
		return Manifest{}, err
	}
	outputDirectory := filepath.Dir(options.OutputPath)
	if err := os.MkdirAll(outputDirectory, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("create backup directory: %w", err)
	}
	workspace, err := os.MkdirTemp(outputDirectory, ".peerphonic-backup-work-*")
	if err != nil {
		return Manifest{}, fmt.Errorf("create backup workspace: %w", err)
	}
	defer os.RemoveAll(workspace)

	snapshotPath := filepath.Join(workspace, "peerphonic.db")
	if err := snapshotter.Snapshot(ctx, snapshotPath); err != nil {
		return Manifest{}, fmt.Errorf("snapshot metadata: %w", err)
	}
	createdAt := time.Now().UTC()
	files := []archiveFile{
		{name: "peerphonic.db", path: snapshotPath},
		{name: "peerphonic.db.auth.key", path: options.CredentialKeyPath},
	}
	manifest := Manifest{FormatVersion: FormatVersion, CreatedAt: createdAt.Format(time.RFC3339Nano)}
	for _, file := range files {
		entry, err := inspectFile(file)
		if err != nil {
			return Manifest{}, err
		}
		manifest.Files = append(manifest.Files, entry)
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, fmt.Errorf("encode backup manifest: %w", err)
	}
	manifestJSON = append(manifestJSON, '\n')
	if err := writeArchive(ctx, options.OutputPath, createdAt, manifestJSON, files); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

type archiveFile struct {
	name string
	path string
}

func ensureMissing(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("backup output %q already exists", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect backup output: %w", err)
	}
	return nil
}

func ensureCredentialKey(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat credential key: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != 32 {
		return fmt.Errorf("credential key %q must be a 32-byte regular file", path)
	}
	return nil
}

func inspectFile(file archiveFile) (ManifestFile, error) {
	source, err := os.Open(file.path)
	if err != nil {
		return ManifestFile{}, fmt.Errorf("open backup file %q: %w", file.name, err)
	}
	digest := sha256.New()
	size, copyErr := io.Copy(digest, source)
	closeErr := source.Close()
	if copyErr != nil {
		return ManifestFile{}, fmt.Errorf("hash backup file %q: %w", file.name, copyErr)
	}
	if closeErr != nil {
		return ManifestFile{}, fmt.Errorf("close backup file %q: %w", file.name, closeErr)
	}
	return ManifestFile{Name: file.name, Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

func writeArchive(
	ctx context.Context,
	outputPath string,
	createdAt time.Time,
	manifest []byte,
	files []archiveFile,
) error {
	temporary, err := os.CreateTemp(filepath.Dir(outputPath), ".peerphonic-backup-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temporary backup archive: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect backup archive: %w", err)
	}
	compressed := gzip.NewWriter(temporary)
	compressed.Header.ModTime = createdAt
	archive := tar.NewWriter(compressed)
	writeErr := writeBytes(archive, "manifest.json", manifest, createdAt)
	for _, file := range files {
		if writeErr == nil {
			writeErr = writeFile(ctx, archive, file, createdAt)
		}
	}
	if err := archive.Close(); writeErr == nil {
		writeErr = err
	}
	if err := compressed.Close(); writeErr == nil {
		writeErr = err
	}
	if err := temporary.Sync(); writeErr == nil {
		writeErr = err
	}
	if err := temporary.Close(); writeErr == nil {
		writeErr = err
	}
	if writeErr != nil {
		return fmt.Errorf("write backup archive: %w", writeErr)
	}
	if err := os.Link(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("commit backup archive: %w", err)
	}
	return nil
}

func writeBytes(archive *tar.Writer, name string, contents []byte, modified time.Time) error {
	header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(contents)), ModTime: modified}
	if err := archive.WriteHeader(header); err != nil {
		return err
	}
	_, err := archive.Write(contents)
	return err
}

func writeFile(ctx context.Context, archive *tar.Writer, file archiveFile, modified time.Time) error {
	source, err := os.Open(file.path)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	header := &tar.Header{Name: file.name, Mode: 0o600, Size: info.Size(), ModTime: modified}
	if err := archive.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(archive, contextReader{ctx: ctx, reader: source})
	return err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
