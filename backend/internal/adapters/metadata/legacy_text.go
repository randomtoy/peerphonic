package metadata

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"
)

// normalizeLegacyText repairs CP1251 bytes stored in ID3 fields marked as
// ISO-8859-1. The replacement is deliberately conservative so legitimate
// Western European text remains unchanged.
func normalizeLegacyText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if candidate, ok := decodeMacCyrillicCP866(value); ok {
		return candidate
	}
	if containsCyrillic(value) {
		return value
	}

	encoded, highBytes, ok := legacyBytes(value)
	if !ok || highBytes < 2 {
		return value
	}
	decoded, err := charmap.Windows1251.NewDecoder().Bytes(encoded)
	if err != nil {
		return value
	}
	candidate := strings.TrimSpace(string(decoded))
	if !preferCyrillicCandidate(value, candidate) {
		return value
	}
	return candidate
}

// decodeMacCyrillicCP866 repairs DOS Cyrillic bytes that were interpreted as
// Macintosh Cyrillic while extracting an archive. macOS may additionally
// decompose accented Cyrillic runes in filenames, so compose them first.
func decodeMacCyrillicCP866(value string) (string, bool) {
	sourceMarkers := mojibakeMarkerCount(value)
	if sourceMarkers < 2 {
		return "", false
	}
	encoded, err := charmap.MacintoshCyrillic.NewEncoder().Bytes([]byte(norm.NFC.String(value)))
	if err != nil {
		return "", false
	}
	decoded, err := charmap.CodePage866.NewDecoder().Bytes(encoded)
	if err != nil {
		return "", false
	}
	candidate := strings.TrimSpace(string(decoded))
	if candidate == "" || candidate == value || cyrillicLetterCount(candidate) < 2 {
		return "", false
	}
	if mojibakeMarkerCount(candidate) >= sourceMarkers || containsUnsafeText(candidate) {
		return "", false
	}
	return candidate, true
}

func mojibakeMarkerCount(value string) int {
	count := 0
	for _, character := range value {
		if unicode.Is(unicode.Mn, character) {
			count++
			continue
		}
		switch character {
		case '†', '™', '®', '¬', '√', '∞':
			count++
		}
	}
	return count
}

func cyrillicLetterCount(value string) int {
	count := 0
	for _, character := range value {
		if unicode.IsLetter(character) && unicode.In(character, unicode.Cyrillic) {
			count++
		}
	}
	return count
}

func containsUnsafeText(value string) bool {
	for _, character := range value {
		if character == unicode.ReplacementChar || unicode.IsControl(character) && !unicode.IsSpace(character) {
			return true
		}
	}
	return false
}

func legacyBytes(value string) ([]byte, int, bool) {
	if !utf8.ValidString(value) {
		highBytes := 0
		for _, value := range []byte(value) {
			if value >= utf8.RuneSelf {
				highBytes++
			}
		}
		return []byte(value), highBytes, true
	}

	encoded := make([]byte, 0, len(value))
	highBytes := 0
	for _, value := range value {
		if value > 0xff {
			return nil, 0, false
		}
		encoded = append(encoded, byte(value))
		if value >= utf8.RuneSelf {
			highBytes++
		}
	}
	return encoded, highBytes, true
}

func preferCyrillicCandidate(source, candidate string) bool {
	letters := 0
	cyrillic := 0
	nonASCII := 0
	for _, value := range candidate {
		if !unicode.IsLetter(value) {
			continue
		}
		letters++
		if value >= utf8.RuneSelf {
			nonASCII++
		}
		if unicode.In(value, unicode.Cyrillic) {
			cyrillic++
		}
	}
	if cyrillic >= 2 && cyrillic*2 >= letters {
		return true
	}
	return cyrillic >= 4 && cyrillic*2 >= nonASCII && longestLegacyByteRun(source) >= 4
}

func longestLegacyByteRun(value string) int {
	longest := 0
	current := 0
	for _, character := range value {
		if character >= utf8.RuneSelf && character <= 0xff {
			current++
			longest = max(longest, current)
			continue
		}
		current = 0
	}
	return longest
}

func containsCyrillic(value string) bool {
	for _, value := range value {
		if unicode.In(value, unicode.Cyrillic) {
			return true
		}
	}
	return false
}

func usableMetadataText(value string) (string, bool) {
	value = normalizeLegacyText(value)
	if value == "" {
		return "", false
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			return value, true
		}
	}
	return "", false
}
