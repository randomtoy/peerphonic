package metadata

import (
	"bytes"
	"encoding/binary"
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

func TestFallbackMetadataRepairsArchiveFilenameEncoding(t *testing.T) {
	t.Parallel()

	got := fallbackMetadata(filepath.Join("Music",
		"Hard covers of fucking pops part20 (б†ђЃѓ†Ђ)",
		"13 - Amatory - Я СЃиЂ† С Уђ†.mp3"))
	if got.Title != "Я Сошла С Ума" || got.Artist != "Amatory" ||
		got.Album != "Hard covers of fucking pops part20 (самопал)" {
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
		"Hard Covers Of Fucking Pops (from Ëèöåìåð)":     "Hard Covers Of Fucking Pops (from Лицемер)",
		"Hard covers of fucking pops part20 (б†ђЃѓ†Ђ)": "Hard covers of fucking pops part20 (самопал)",
		"Я СЃиЂ† С Уђ†":                                 "Я Сошла С Ума",
		"Овѓгб™†о (М†™S®ђ cover Live)":                  "Отпускаю (МакSим cover Live)",
		"Кукрыниксы":                                     "Кукрыниксы",
		"Ѓорѓи":                                          "Ѓорѓи",
		"Koßn":                                           "Koßn",
		"Beyoncé déjà vu":                                "Beyoncé déjà vu",
		"A'party'ÿ":                                      "A'party'ÿ",
		"Therapy?":                                       "Therapy?",
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
	audio := append(bytes.Repeat(mp3.SilentBytes, 10), id3v1Tag(t, "Песня", "Черная Метка", "Альбом", 17)...)
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
	if got.Title != "Песня" || got.Artist != "Черная Метка" || got.Album != "Альбом" ||
		got.AlbumArtist != "Черная Метка" || got.Genre != "Rock" {
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

func TestTagExtractorPrefersEmbeddedArtwork(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	embedded := append([]byte(nil), testPNGBytes...)
	embedded = append(embedded, "embedded"...)
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("folder artwork"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "track.mp3")
	audio := append(id3v23PictureTag(embedded), bytes.Repeat(mp3.SilentBytes, 10)...)
	if err := os.WriteFile(path, audio, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (TagExtractor{}).Extract(path, info)
	if err != nil {
		t.Fatal(err)
	}
	if got.Artwork == nil || !bytes.Equal(got.Artwork.Data, embedded) {
		t.Fatalf("artwork = %#v", got.Artwork)
	}
}

func TestTagExtractorUsesFolderArtwork(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cover := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	if err := os.WriteFile(filepath.Join(dir, "cover 13.jpg"), cover, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(path, bytes.Repeat(mp3.SilentBytes, 10), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (TagExtractor{}).Extract(path, info)
	if err != nil {
		t.Fatal(err)
	}
	if got.Artwork == nil || !bytes.Equal(got.Artwork.Data, cover) {
		t.Fatalf("artwork = %#v", got.Artwork)
	}
}

var testPNGBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func id3v23PictureTag(picture []byte) []byte {
	payload := []byte{0}
	payload = append(payload, "image/png"...)
	payload = append(payload, 0, 3, 0)
	payload = append(payload, picture...)
	frame := []byte("APIC")
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(len(payload)))
	frame = append(frame, size...)
	frame = append(frame, 0, 0)
	frame = append(frame, payload...)
	header := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, byte(len(frame))}
	return append(header, frame...)
}

func id3v1Tag(t *testing.T, title, artist, album string, genre ...byte) []byte {
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
	if len(genre) > 0 {
		tag[127] = genre[0]
	}
	return tag
}
