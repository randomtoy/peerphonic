package filesystem

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Put(ctx, "tracks/example", strings.NewReader("audio")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	stream, err := store.Open(ctx, "tracks/example")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	data, _ := io.ReadAll(stream)
	stream.Close()
	if string(data) != "audio" {
		t.Fatalf("content = %q, want audio", data)
	}
	if err := store.Delete(ctx, "tracks/example"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestStoreRejectsTraversal(t *testing.T) {
	t.Parallel()

	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "../outside", strings.NewReader("no")); err == nil {
		t.Fatal("Put() error = nil, want traversal error")
	}
}
