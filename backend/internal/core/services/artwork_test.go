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
	service := NewArtworkService(store, artworkSourceStub{data: external})
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

type artworkSourceStub struct {
	data []byte
}

func (s artworkSourceStub) OpenArtwork(_ context.Context, _ string) (ports.ResolvedSource, error) {
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
	t.Helper()
	var data bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
