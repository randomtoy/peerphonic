package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type Store struct {
	root string
}

func New(root string) (*Store, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve blob root: %w", err)
	}
	return &Store{root: absolute}, nil
}

func (s *Store) Open(_ context.Context, key string) (ports.ReadSeekCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("open blob: %w", err)
	}
	return file, nil
}

func (s *Store) Put(ctx context.Context, key string, source io.Reader) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create blob directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".peerphonic-*")
	if err != nil {
		return fmt.Errorf("create temporary blob: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	reader := &contextReader{ctx: ctx, reader: source}
	if _, err := io.Copy(temporary, reader); err != nil {
		temporary.Close()
		return fmt.Errorf("write blob: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync blob: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close blob: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("commit blob: %w", err)
	}
	return nil
}

func (s *Store) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ports.ErrNotFound
		}
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

func (s *Store) path(key string) (string, error) {
	if key == "" || filepath.IsAbs(key) {
		return "", fmt.Errorf("invalid blob key %q", key)
	}
	relative := filepath.Clean(filepath.FromSlash(key))
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("blob key escapes storage root: %q", key)
	}
	return filepath.Join(s.root, relative), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
