package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type ArtworkService struct {
	blobs ports.BlobStore
}

func NewArtworkService(blobs ports.BlobStore) *ArtworkService {
	return &ArtworkService{blobs: blobs}
}

func (s *ArtworkService) Put(ctx context.Context, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("artwork is empty")
	}
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("unsupported artwork content type %q", contentType)
	}
	id := domain.StableID("art", string(data))
	if err := s.blobs.Put(ctx, artworkKey(id), bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("store artwork: %w", err)
	}
	return id, nil
}

func (s *ArtworkService) Open(ctx context.Context, id string) (ports.ResolvedSource, error) {
	if !validArtworkID(id) {
		return ports.ResolvedSource{}, ports.ErrNotFound
	}
	content, err := s.blobs.Open(ctx, artworkKey(id))
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return ports.ResolvedSource{}, ports.ErrNotFound
		}
		return ports.ResolvedSource{}, fmt.Errorf("open artwork: %w", err)
	}
	fail := func(err error) (ports.ResolvedSource, error) {
		content.Close()
		return ports.ResolvedSource{}, err
	}

	header := make([]byte, 512)
	read, err := content.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return fail(fmt.Errorf("read artwork header: %w", err))
	}
	contentType := http.DetectContentType(header[:read])
	if !strings.HasPrefix(contentType, "image/") {
		return fail(fmt.Errorf("stored artwork has content type %q", contentType))
	}
	size, err := content.Seek(0, io.SeekEnd)
	if err != nil {
		return fail(fmt.Errorf("read artwork size: %w", err))
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return fail(fmt.Errorf("rewind artwork: %w", err))
	}
	return ports.ResolvedSource{
		Content: content, Name: id, ContentType: contentType, Size: size,
	}, nil
}

func artworkKey(id string) string {
	return "artwork/" + id
}

func validArtworkID(id string) bool {
	return strings.HasPrefix(id, "art_") && !strings.ContainsAny(id, `/\\`)
}
