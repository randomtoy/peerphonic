package filesystem

import (
	"context"
	"errors"
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

func TestStoreKeepsPreviousBlobWhenReplacementIsCancelled(t *testing.T) {
	t.Parallel()

	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "tracks/example", strings.NewReader("original")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(ctx, "tracks/example", strings.NewReader("replacement")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put() error = %v, want context.Canceled", err)
	}
	stream, err := store.Open(context.Background(), "tracks/example")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("content = %q, want original", got)
	}
}
