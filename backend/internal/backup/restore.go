package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const manifestSizeLimit = 1 << 20

type RestoreOptions struct {
	InputPath         string
	DatabasePath      string
	CredentialKeyPath string
	Force             bool
}

type RestoreResult struct {
	Manifest Manifest
	Replaced []string
}

// Restore validates a complete backup before replacing any live files. It is
// intended for offline use: the server must not have the target database open.
func Restore(ctx context.Context, options RestoreOptions) (RestoreResult, error) {
	if options.InputPath == "" || options.DatabasePath == "" || options.CredentialKeyPath == "" {
		return RestoreResult{}, fmt.Errorf("backup input, database, and credential key paths are required")
	}
	if err := os.MkdirAll(filepath.Dir(options.DatabasePath), 0o700); err != nil {
		return RestoreResult{}, fmt.Errorf("create restore workspace directory: %w", err)
	}
	workspace, err := os.MkdirTemp(filepath.Dir(options.DatabasePath), ".peerphonic-restore-work-*")
	if err != nil {
		return RestoreResult{}, fmt.Errorf("create restore workspace: %w", err)
	}
	defer os.RemoveAll(workspace)

	manifest, extracted, err := extractAndVerify(ctx, options.InputPath, workspace)
	if err != nil {
		return RestoreResult{}, err
	}
	targets := []restoreTarget{
		{source: extracted["peerphonic.db"], destination: options.DatabasePath},
		{source: extracted["peerphonic.db.auth.key"], destination: options.CredentialKeyPath},
	}
	if err := prepareTargets(targets, options.Force); err != nil {
		return RestoreResult{}, err
	}
	replaced, err := commitTargets(ctx, targets, options.Force)
	if err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{Manifest: manifest, Replaced: replaced}, nil
}

type restoreTarget struct {
	source      string
	destination string
}

func extractAndVerify(ctx context.Context, inputPath, workspace string) (Manifest, map[string]string, error) {
	input, err := os.Open(inputPath)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open backup archive: %w", err)
	}
	defer input.Close()
	compressed, err := gzip.NewReader(input)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open compressed backup: %w", err)
	}
	defer compressed.Close()

	allowed := map[string]bool{"manifest.json": true, "peerphonic.db": true, "peerphonic.db.auth.key": true}
	extracted := make(map[string]string)
	var manifestJSON []byte
	archive := tar.NewReader(compressed)
	for {
		if err := ctx.Err(); err != nil {
			return Manifest{}, nil, err
		}
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("read backup archive: %w", err)
		}
		if !allowed[header.Name] || header.Typeflag != tar.TypeReg || extracted[header.Name] != "" || (header.Name == "manifest.json" && manifestJSON != nil) {
			return Manifest{}, nil, fmt.Errorf("unexpected or duplicate backup entry %q", header.Name)
		}
		if header.Name == "manifest.json" {
			if header.Size < 0 || header.Size > manifestSizeLimit {
				return Manifest{}, nil, fmt.Errorf("backup manifest has invalid size %d", header.Size)
			}
			manifestJSON, err = io.ReadAll(io.LimitReader(archive, manifestSizeLimit+1))
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("read backup manifest: %w", err)
			}
			if int64(len(manifestJSON)) != header.Size {
				return Manifest{}, nil, fmt.Errorf("read backup manifest: copied %d of %d bytes", len(manifestJSON), header.Size)
			}
			continue
		}
		path := filepath.Join(workspace, header.Name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("create restored %q: %w", header.Name, err)
		}
		written, copyErr := io.Copy(file, contextReader{ctx: ctx, reader: archive})
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || written != header.Size {
			return Manifest{}, nil, fmt.Errorf("extract backup entry %q: copied %d of %d bytes: %v", header.Name, written, header.Size, firstError(copyErr, closeErr))
		}
		extracted[header.Name] = path
	}
	if manifestJSON == nil {
		return Manifest{}, nil, fmt.Errorf("backup manifest is missing")
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("decode backup manifest: %w", err)
	}
	if manifest.FormatVersion != FormatVersion {
		return Manifest{}, nil, fmt.Errorf("unsupported backup format version %d", manifest.FormatVersion)
	}
	if len(manifest.Files) != 2 {
		return Manifest{}, nil, fmt.Errorf("backup manifest must contain exactly two files")
	}
	seen := make(map[string]bool)
	for _, entry := range manifest.Files {
		path := extracted[entry.Name]
		if path == "" || seen[entry.Name] || (entry.Name != "peerphonic.db" && entry.Name != "peerphonic.db.auth.key") {
			return Manifest{}, nil, fmt.Errorf("invalid backup manifest entry %q", entry.Name)
		}
		seen[entry.Name] = true
		actual, err := inspectFile(archiveFile{name: entry.Name, path: path})
		if err != nil {
			return Manifest{}, nil, err
		}
		if actual.Size != entry.Size || actual.SHA256 != entry.SHA256 {
			return Manifest{}, nil, fmt.Errorf("backup entry %q failed checksum verification", entry.Name)
		}
	}
	if !seen["peerphonic.db"] || !seen["peerphonic.db.auth.key"] {
		return Manifest{}, nil, fmt.Errorf("backup is incomplete")
	}
	return manifest, extracted, nil
}

func prepareTargets(targets []restoreTarget, force bool) error {
	for _, target := range targets {
		if err := os.MkdirAll(filepath.Dir(target.destination), 0o700); err != nil {
			return fmt.Errorf("create restore target directory: %w", err)
		}
		info, err := os.Lstat(target.destination)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect restore target %q: %w", target.destination, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("restore target %q is not a regular file", target.destination)
		}
		if !force {
			return fmt.Errorf("restore target %q already exists; stop the server and use --force to replace it", target.destination)
		}
	}
	return nil
}

func commitTargets(ctx context.Context, targets []restoreTarget, force bool) ([]string, error) {
	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	staged := make([]string, len(targets))
	for index, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, err := os.CreateTemp(filepath.Dir(target.destination), ".peerphonic-restore-*")
		if err != nil {
			return nil, fmt.Errorf("stage restore target: %w", err)
		}
		staged[index] = file.Name()
		defer os.Remove(file.Name())
		if err := file.Chmod(0o600); err != nil {
			file.Close()
			return nil, err
		}
		source, err := os.Open(target.source)
		if err != nil {
			file.Close()
			return nil, err
		}
		_, copyErr := io.Copy(file, contextReader{ctx: ctx, reader: source})
		closeSourceErr := source.Close()
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := firstError(copyErr, closeSourceErr, syncErr, closeErr); err != nil {
			return nil, fmt.Errorf("stage restore target %q: %w", target.destination, err)
		}
	}

	backups := make(map[string]string)
	if force {
		for _, target := range targets {
			if _, err := os.Lstat(target.destination); os.IsNotExist(err) {
				continue
			} else if err != nil {
				rollbackBackups(backups)
				return nil, err
			}
			backupPath := target.destination + ".before-restore-" + timestamp
			if err := os.Rename(target.destination, backupPath); err != nil {
				rollbackBackups(backups)
				return nil, fmt.Errorf("preserve existing restore target %q: %w", target.destination, err)
			}
			backups[target.destination] = backupPath
		}
	}
	committed := make([]string, 0, len(targets))
	for index, target := range targets {
		var err error
		if force {
			err = os.Rename(staged[index], target.destination)
		} else {
			err = os.Link(staged[index], target.destination)
			if err == nil {
				err = os.Remove(staged[index])
			}
		}
		if err != nil {
			for _, path := range committed {
				_ = os.Remove(path)
			}
			rollbackBackups(backups)
			return nil, fmt.Errorf("commit restore target %q: %w", target.destination, err)
		}
		committed = append(committed, target.destination)
	}
	replaced := make([]string, 0, len(backups))
	for _, path := range backups {
		replaced = append(replaced, path)
	}
	sort.Strings(replaced)
	return replaced, nil
}

func rollbackBackups(backups map[string]string) {
	for destination, backupPath := range backups {
		_ = os.Rename(backupPath, destination)
	}
}

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}
