package services

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/blob/filesystem"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestArtworkServiceStoresAndOpensContentAddressedImage(t *testing.T) {
	t.Parallel()

	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewArtworkService(store)
	imageData := testPNG(t)
	id, err := service.Put(context.Background(), imageData)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Put(context.Background(), imageData)
	if err != nil || again != id {
		t.Fatalf("second Put() = %q, %v, want %q", again, err, id)
	}

	resolved, err := service.Open(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer resolved.Content.Close()
	got, err := io.ReadAll(resolved.Content)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, imageData) || resolved.ContentType != "image/png" || resolved.Size != int64(len(imageData)) {
		t.Fatalf("resolved artwork = %#v, bytes = %d", resolved, len(got))
	}
}

func TestArtworkServiceRejectsNonImagesAndInvalidIDs(t *testing.T) {
	t.Parallel()

	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewArtworkService(store)
	if _, err := service.Put(context.Background(), []byte("not an image")); err == nil {
		t.Fatal("Put() error = nil, want unsupported content type")
	}
	if _, err := service.Open(context.Background(), "../audio"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Open() error = %v, want ErrNotFound", err)
	}
}

func TestArtworkServiceFallsBackToProviderArtwork(t *testing.T) {
	t.Parallel()

	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	external := testPNG(t)
	service := NewArtworkService(store, &artworkSourceStub{data: external})
	resolved, err := service.Open(context.Background(), "torrentart_example")
	if err != nil {
		t.Fatal(err)
	}
	defer resolved.Content.Close()
	contents, err := io.ReadAll(resolved.Content)
	if err != nil || !bytes.Equal(contents, external) {
		t.Fatalf("fallback artwork bytes = %d, err = %v", len(contents), err)
	}
}

func TestArtworkServiceResizesAndCachesProviderArtwork(t *testing.T) {
	t.Parallel()

	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	external := sizedPNG(t, 120, 60)
	source := &artworkSourceStub{data: external}
	service := NewArtworkService(store, source)
	for attempt := 0; attempt < 2; attempt++ {
		resolved, err := service.OpenSized(context.Background(), "torrentart_large", 30)
		if err != nil {
			t.Fatal(err)
		}
		config, _, err := image.DecodeConfig(resolved.Content)
		resolved.Content.Close()
		if err != nil {
			t.Fatal(err)
		}
		if config.Width != 30 || config.Height != 15 || resolved.ContentType != "image/png" {
			t.Fatalf("resized artwork = %dx%d, %q", config.Width, config.Height, resolved.ContentType)
		}
	}
	if source.calls != 1 {
		t.Fatalf("OpenArtwork() calls = %d, want 1", source.calls)
	}
}

type artworkSourceStub struct {
	data  []byte
	calls int
}

func (s *artworkSourceStub) OpenArtwork(_ context.Context, _ string) (ports.ResolvedSource, error) {
	s.calls++
	return ports.ResolvedSource{
		Content: &readSeekCloser{Reader: bytes.NewReader(s.data)}, ContentType: "image/png",
		Size: int64(len(s.data)),
	}, nil
}

type readSeekCloser struct {
	*bytes.Reader
}

func (*readSeekCloser) Close() error { return nil }

func testPNG(t *testing.T) []byte {
	return sizedPNG(t, 2, 2)
}

func sizedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var data bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
