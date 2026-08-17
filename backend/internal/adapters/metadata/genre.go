package metadata

import (
	"strings"
	"unicode"
)

func normalizeGenre(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !containsLetter(value) {
		return ""
	}
	words := strings.Fields(value)
	if len(words)%2 != 0 {
		return value
	}
	half := len(words) / 2
	for index := 0; index < half; index++ {
		if !strings.EqualFold(words[index], words[index+half]) {
			return value
		}
	}
	return strings.Join(words[:half], " ")
}

func containsLetter(value string) bool {
	for _, character := range value {
		if unicode.IsLetter(character) {
			return true
		}
	}
	return false
}
