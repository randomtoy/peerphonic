package metadata

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tcolgate/mp3"
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

	path := filepath.Join(t.TempDir(), "untagged.mp3")
	if err := os.WriteFile(path, bytes.Repeat(mp3.SilentBytes, 10), 0o600); err != nil {
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
	if got.Title != "untagged" || got.Suffix != "mp3" || got.ContentType != "audio/mpeg" {
		t.Fatalf("Extract() = %#v", got)
	}
	wantBitRate := int(mp3.SilentFrame.Header().BitRate()) / 1000
	if got.Duration <= 0 || got.BitRate != wantBitRate {
		t.Fatalf("audio properties = %v, %d kbps", got.Duration, got.BitRate)
	}
}
