package metadata

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dhowden/tag"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

const maxArtworkSize = 32 << 20

func extractArtwork(audioPath string, values tag.Metadata) *scanner.Artwork {
	if values != nil {
		if picture := values.Picture(); picture != nil && len(picture.Data) > 0 {
			return &scanner.Artwork{Data: append([]byte(nil), picture.Data...)}
		}
	}
	return folderArtwork(filepath.Dir(audioPath))
}

func folderArtwork(directory string) *scanner.Artwork {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	type candidate struct {
		path     string
		priority int
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !isArtworkExtension(filepath.Ext(entry.Name())) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxArtworkSize {
			continue
		}
		candidates = append(candidates, candidate{
			path: filepath.Join(directory, entry.Name()), priority: artworkPriority(entry.Name()),
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		return candidates[i].path < candidates[j].path
	})
	if candidates[0].priority == 2 && len(candidates) != 1 {
		return nil
	}
	data, err := os.ReadFile(candidates[0].path)
	if err != nil {
		return nil
	}
	return &scanner.Artwork{Data: data}
}

func isArtworkExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func artworkPriority(name string) int {
	stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	switch stem {
	case "cover", "folder", "front", "albumart":
		return 0
	}
	for _, prefix := range []string{"cover", "folder", "front", "albumart"} {
		if strings.HasPrefix(stem, prefix) {
			return 1
		}
	}
	return 2
}
