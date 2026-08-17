package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type ArtworkService struct {
	blobs     ports.BlobStore
	fallbacks []ArtworkSource
}

type ArtworkSource interface {
	OpenArtwork(ctx context.Context, id string) (ports.ResolvedSource, error)
}

func NewArtworkService(blobs ports.BlobStore, fallbacks ...ArtworkSource) *ArtworkService {
	return &ArtworkService{blobs: blobs, fallbacks: fallbacks}
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
	if validArtworkID(id) {
		resolved, err := s.openStored(ctx, id)
		if err == nil || !errors.Is(err, ports.ErrNotFound) {
			return resolved, err
		}
	}
	for _, fallback := range s.fallbacks {
		resolved, err := fallback.OpenArtwork(ctx, id)
		if err == nil || !errors.Is(err, ports.ErrNotFound) {
			return resolved, err
		}
	}
	return ports.ResolvedSource{}, ports.ErrNotFound
}

// OpenSized returns artwork fitted within a square bounding box. Generated
// variants are stored in blob storage so provider artwork is only fetched and
// resized once for each requested size.
func (s *ArtworkService) OpenSized(ctx context.Context, id string, size int) (ports.ResolvedSource, error) {
	if size <= 0 {
		return s.Open(ctx, id)
	}
	variantID := domain.StableID("thumb", id, strconv.Itoa(size))
	if resolved, err := s.openStoredKey(ctx, artworkKey(variantID), variantID); err == nil {
		return resolved, nil
	} else if !errors.Is(err, ports.ErrNotFound) {
		return ports.ResolvedSource{}, err
	}

	original, err := s.Open(ctx, id)
	if err != nil {
		return ports.ResolvedSource{}, err
	}
	config, _, err := image.DecodeConfig(original.Content)
	if err != nil {
		original.Content.Close()
		return ports.ResolvedSource{}, fmt.Errorf("inspect artwork dimensions: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 20_000 || config.Height > 20_000 ||
		int64(config.Width)*int64(config.Height) > 100_000_000 {
		original.Content.Close()
		return ports.ResolvedSource{}, fmt.Errorf("artwork dimensions %dx%d exceed resize limits", config.Width, config.Height)
	}
	if config.Width <= size && config.Height <= size {
		if _, err := original.Content.Seek(0, io.SeekStart); err != nil {
			original.Content.Close()
			return ports.ResolvedSource{}, fmt.Errorf("rewind artwork: %w", err)
		}
		return original, nil
	}
	if _, err := original.Content.Seek(0, io.SeekStart); err != nil {
		original.Content.Close()
		return ports.ResolvedSource{}, fmt.Errorf("rewind artwork for resize: %w", err)
	}
	picture, format, err := image.Decode(original.Content)
	original.Content.Close()
	if err != nil {
		return ports.ResolvedSource{}, fmt.Errorf("decode artwork for resize: %w", err)
	}
	resized := resizeArtwork(picture, size)
	var encoded bytes.Buffer
	contentType := "image/png"
	if format == "jpeg" {
		contentType = "image/jpeg"
		err = jpeg.Encode(&encoded, resized, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&encoded, resized)
	}
	if err != nil {
		return ports.ResolvedSource{}, fmt.Errorf("encode resized artwork: %w", err)
	}
	data := encoded.Bytes()
	key := artworkKey(variantID)
	if err := s.blobs.Put(ctx, key, bytes.NewReader(data)); err == nil {
		return s.openStoredKey(ctx, key, variantID)
	}
	return ports.ResolvedSource{
		Content: &memoryReadSeekCloser{Reader: bytes.NewReader(data)}, Name: variantID,
		ContentType: contentType, Size: int64(len(data)),
	}, nil
}

func (s *ArtworkService) openStored(ctx context.Context, id string) (ports.ResolvedSource, error) {
	return s.openStoredKey(ctx, artworkKey(id), id)
}

func (s *ArtworkService) openStoredKey(ctx context.Context, key, name string) (ports.ResolvedSource, error) {
	content, err := s.blobs.Open(ctx, key)
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
		Content: content, Name: name, ContentType: contentType, Size: size,
	}, nil
}

func resizeArtwork(source image.Image, maxDimension int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	scale := math.Min(float64(maxDimension)/float64(width), float64(maxDimension)/float64(height))
	targetWidth := max(1, int(math.Round(float64(width)*scale)))
	targetHeight := max(1, int(math.Round(float64(height)*scale)))
	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := range targetHeight {
		sourceY := (float64(y)+0.5)*float64(height)/float64(targetHeight) - 0.5
		y0, y1, yWeight := interpolationCoordinates(sourceY, height)
		for x := range targetWidth {
			sourceX := (float64(x)+0.5)*float64(width)/float64(targetWidth) - 0.5
			x0, x1, xWeight := interpolationCoordinates(sourceX, width)
			target.SetRGBA64(x, y, interpolateColor(
				source.At(bounds.Min.X+x0, bounds.Min.Y+y0),
				source.At(bounds.Min.X+x1, bounds.Min.Y+y0),
				source.At(bounds.Min.X+x0, bounds.Min.Y+y1),
				source.At(bounds.Min.X+x1, bounds.Min.Y+y1),
				xWeight, yWeight,
			))
		}
	}
	return target
}

func interpolationCoordinates(value float64, length int) (lower, upper int, weight float64) {
	lower = int(math.Floor(value))
	weight = value - float64(lower)
	if lower < 0 {
		return 0, 0, 0
	}
	if lower >= length-1 {
		return length - 1, length - 1, 0
	}
	return lower, lower + 1, weight
}

func interpolateColor(topLeft, topRight, bottomLeft, bottomRight color.Color, xWeight, yWeight float64) color.RGBA64 {
	tlR, tlG, tlB, tlA := topLeft.RGBA()
	trR, trG, trB, trA := topRight.RGBA()
	blR, blG, blB, blA := bottomLeft.RGBA()
	brR, brG, brB, brA := bottomRight.RGBA()
	return color.RGBA64{
		R: interpolateChannel(tlR, trR, blR, brR, xWeight, yWeight),
		G: interpolateChannel(tlG, trG, blG, brG, xWeight, yWeight),
		B: interpolateChannel(tlB, trB, blB, brB, xWeight, yWeight),
		A: interpolateChannel(tlA, trA, blA, brA, xWeight, yWeight),
	}
}

func interpolateChannel(topLeft, topRight, bottomLeft, bottomRight uint32, xWeight, yWeight float64) uint16 {
	top := float64(topLeft)*(1-xWeight) + float64(topRight)*xWeight
	bottom := float64(bottomLeft)*(1-xWeight) + float64(bottomRight)*xWeight
	return uint16(math.Round(top*(1-yWeight) + bottom*yWeight))
}

type memoryReadSeekCloser struct {
	*bytes.Reader
}

func (*memoryReadSeekCloser) Close() error { return nil }

func artworkKey(id string) string {
	return "artwork/" + id
}

func validArtworkID(id string) bool {
	return strings.HasPrefix(id, "art_") && !strings.ContainsAny(id, `/\\`)
}
