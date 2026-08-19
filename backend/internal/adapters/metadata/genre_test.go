package metadata

import "testing"

func TestNormalizeGenre(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":                        "",
		"05":                      "",
		"(22)":                    "",
		"Death Metal Death Metal": "Death Metal",
		"rock ROCK":               "rock",
		"Rock":                    "Rock",
		"2 Tone":                  "2 Tone",
		"Rhythm and Blues":        "Rhythm and Blues",
		"Alternative Rock / Pop":  "Alternative Rock / Pop",
		"Русский рок Русский РОК":   "Русский рок",
		"Electronic Electronic Pop": "Electronic Electronic Pop",
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if got := normalizeGenre(input); got != want {
				t.Fatalf("normalizeGenre(%q) = %q, want %q", input, got, want)
			}
		})
	}
}
