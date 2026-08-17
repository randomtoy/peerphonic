package metadata

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// normalizeLegacyText repairs CP1251 bytes stored in ID3 fields marked as
// ISO-8859-1. The replacement is deliberately conservative so legitimate
// Western European text remains unchanged.
func normalizeLegacyText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || containsCyrillic(value) {
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
	if !preferCyrillicCandidate(candidate) {
		return value
	}
	return candidate
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

func preferCyrillicCandidate(value string) bool {
	letters := 0
	cyrillic := 0
	for _, value := range value {
		if !unicode.IsLetter(value) {
			continue
		}
		letters++
		if unicode.In(value, unicode.Cyrillic) {
			cyrillic++
		}
	}
	return cyrillic >= 2 && cyrillic*2 >= letters
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
