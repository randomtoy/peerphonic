package domain

import "testing"

func TestStableID(t *testing.T) {
	t.Parallel()

	first := StableID("track", "local", "/music/song.mp3")
	second := StableID("track", "local", "/music/song.mp3")
	other := StableID("track", "local", "/music/other.mp3")

	if first != second {
		t.Fatalf("StableID is not deterministic: %q != %q", first, second)
	}
	if first == other {
		t.Fatalf("StableID collision for different input: %q", first)
	}
}

func TestCanonicalCatalogIdentityIgnoresDisplayVariants(t *testing.T) {
	t.Parallel()

	artistVariants := []string{
		"Ron Pope",
		"ron pope",
		"  RON\u00a0\u00a0POPE  ",
		"Ｒｏｎ Ｐｏｐｅ",
	}
	wantArtistID := CanonicalArtistID(artistVariants[0])
	for _, variant := range artistVariants[1:] {
		if got := CanonicalArtistID(variant); got != wantArtistID {
			t.Errorf("CanonicalArtistID(%q) = %q, want %q", variant, got, wantArtistID)
		}
	}

	wantAlbumID := CanonicalAlbumID("System of a Down", "Toxicity")
	if got := CanonicalAlbumID("SYSTEM OF A DOWN", " toxicity "); got != wantAlbumID {
		t.Fatalf("CanonicalAlbumID() = %q, want %q", got, wantAlbumID)
	}
}

func TestCanonicalTrackIDIgnoresProviderFilenameFormatting(t *testing.T) {
	t.Parallel()

	local := Track{
		Title: "Prison Song", Artist: "System of a Down", Album: "Toxicity",
		AlbumArtist: "System of a Down", TrackNumber: 1,
	}
	remote := Track{
		Title: "01 - PRISON SONG", Artist: "system of a down", Album: " toxicity ",
		AlbumArtist: "SYSTEM OF A DOWN",
	}
	if got, want := CanonicalTrackID(remote), CanonicalTrackID(local); got != want {
		t.Fatalf("remote track ID = %q, want %q", got, want)
	}
	other := local
	other.Title = "Needles"
	other.TrackNumber = 2
	if CanonicalTrackID(other) == CanonicalTrackID(local) {
		t.Fatal("different album tracks received the same logical ID")
	}
}
