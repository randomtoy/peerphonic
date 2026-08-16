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
