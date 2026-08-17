package metadata

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tcolgate/mp3"
	"golang.org/x/text/encoding/charmap"
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

func TestFallbackMetadataUsesArtistAndTitleFromCompilationFilename(t *testing.T) {
	t.Parallel()

	got := fallbackMetadata(filepath.Join("Music", "Compilations", "Volume 1", "04. Черная Метка - Песня.mp3"))
	if got.Title != "Песня" || got.Artist != "Черная Метка" || got.Album != "Volume 1" {
		t.Fatalf("fallbackMetadata() = %#v", got)
	}
}

func TestFallbackMetadataReplacesLostFilenameArtist(t *testing.T) {
	t.Parallel()

	got := fallbackMetadata(filepath.Join("Music", "Compilations", "Volume 1", "05. _____ - Song.mp3"))
	if got.Title != "Song" || got.Artist != "Unknown Artist" || got.Album != "Volume 1" {
		t.Fatalf("fallbackMetadata() = %#v", got)
	}
}

func TestNormalizeLegacyText(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"×åðíàÿ Ìåòêà":  "Черная Метка",
		"Îäíà Íà Äâîèõ": "Одна На Двоих",
		"Êîìàòîzz":      "Коматоzz",
		"Í.Ý.Ï.":        "Н.Э.П.",
		"Кукрыниксы":    "Кукрыниксы",
		"Koßn":          "Koßn",
		"A'party'ÿ":     "A'party'ÿ",
		"Therapy?":      "Therapy?",
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if got := normalizeLegacyText(input); got != want {
				t.Fatalf("normalizeLegacyText(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestTagExtractorDecodesCP1251ID3v1(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "track.mp3")
	audio := append(bytes.Repeat(mp3.SilentBytes, 10), id3v1Tag(t, "Песня", "Черная Метка", "Альбом")...)
	if err := os.WriteFile(path, audio, 0o600); err != nil {
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
	if got.Title != "Песня" || got.Artist != "Черная Метка" || got.Album != "Альбом" || got.AlbumArtist != "Черная Метка" {
		t.Fatalf("Extract() = %#v", got)
	}
}

func TestTagExtractorFallsBackFromPlaceholderArtist(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "Compilation")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "05. Черная Метка - Песня.mp3")
	audio := append(bytes.Repeat(mp3.SilentBytes, 10), id3v1Tag(t, "Песня", "???????", "VA")...)
	if err := os.WriteFile(path, audio, 0o600); err != nil {
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
	if got.Artist != "Черная Метка" || got.AlbumArtist != "Черная Метка" {
		t.Fatalf("Extract() = %#v", got)
	}
}

func id3v1Tag(t *testing.T, title, artist, album string) []byte {
	t.Helper()
	tag := make([]byte, 128)
	copy(tag, "TAG")
	copyCP1251 := func(offset int, value string) {
		encoded, err := charmap.Windows1251.NewEncoder().Bytes([]byte(value))
		if err != nil {
			t.Fatal(err)
		}
		copy(tag[offset:offset+30], encoded)
	}
	copyCP1251(3, title)
	copyCP1251(33, artist)
	copyCP1251(63, album)
	copy(tag[93:97], "2000")
	tag[127] = 255
	return tag
}
