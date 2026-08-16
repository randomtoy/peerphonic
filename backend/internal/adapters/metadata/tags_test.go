package metadata

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFallbackMetadataUsesDirectoryLayout(t *testing.T) {
	t.Parallel()

	got := fallbackMetadata(filepath.Join("Music", "Artist", "Album", "01 - Song.mp3"))
	if got.Title != "01 - Song" || got.Artist != "Artist" || got.Album != "Album" {
		t.Fatalf("fallbackMetadata() = %#v", got)
	}
}

func TestTagExtractorUsesFallbackWithoutTags(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.mp3")
	if err := os.WriteFile(path, bytes.Repeat([]byte{0}, 256), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (TagExtractor{}).Extract(path, info)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Title != "invalid" || got.Suffix != "mp3" || got.ContentType != "audio/mpeg" {
		t.Fatalf("Extract() = %#v", got)
	}
}
