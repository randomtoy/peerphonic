package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/randomtoy/peerphonic/backend/internal/audioformat"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const Name = "local"

type Provider struct {
	root string
}

func New(root string) (*Provider, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve provider root: %w", err)
	}
	return &Provider{root: absolute}, nil
}

func (p *Provider) Name() string { return Name }

func (p *Provider) Search(context.Context, domain.SearchQuery) ([]domain.TrackSource, error) {
	return nil, nil
}

func (p *Provider) Resolve(_ context.Context, _ string, ref domain.SourceRef) (ports.ResolvedSource, error) {
	if ref.Provider != Name {
		return ports.ResolvedSource{}, fmt.Errorf("cannot resolve provider %q", ref.Provider)
	}
	path, err := securePath(p.root, ref.Key)
	if err != nil {
		return ports.ResolvedSource{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return ports.ResolvedSource{}, fmt.Errorf("open local media: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return ports.ResolvedSource{}, fmt.Errorf("stat local media: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return ports.ResolvedSource{}, fmt.Errorf("local media %q is not a regular file", ref.Key)
	}
	format, _ := audioformat.FromPath(info.Name())
	return ports.ResolvedSource{
		Content: file, Name: info.Name(), ContentType: format.ContentType,
		Size: info.Size(), ModTime: info.ModTime(),
	}, nil
}

func securePath(root, key string) (string, error) {
	if key == "" || filepath.IsAbs(key) {
		return "", fmt.Errorf("invalid local media key %q", key)
	}
	relative := filepath.Clean(filepath.FromSlash(key))
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local media key escapes library root: %q", key)
	}
	return filepath.Join(root, relative), nil
}
