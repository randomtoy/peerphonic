package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func TestResolveLocalTrack(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "Artist", "song.mp3")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := provider.Resolve(context.Background(), "", domain.SourceRef{Provider: Name, Key: "Artist/song.mp3"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	defer resolved.Content.Close()
	data, _ := io.ReadAll(resolved.Content)
	if string(data) != "audio" || resolved.ContentType != "audio/mpeg" {
		t.Fatalf("content = %q, content type = %q", data, resolved.ContentType)
	}
}

func TestResolveRejectsTraversal(t *testing.T) {
	t.Parallel()

	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Resolve(context.Background(), "", domain.SourceRef{Provider: Name, Key: "../secret"})
	if err == nil {
		t.Fatal("Resolve() error = nil, want traversal error")
	}
}
