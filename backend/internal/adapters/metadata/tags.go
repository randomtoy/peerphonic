package metadata

import (
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type TagExtractor struct{}

func (TagExtractor) Extract(path string, info os.FileInfo) (scanner.Metadata, error) {
	extension := strings.ToLower(filepath.Ext(path))
	fallback := fallbackMetadata(path)
	result := scanner.Metadata{
		Title:       fallback.Title,
		Artist:      fallback.Artist,
		Album:       fallback.Album,
		AlbumArtist: fallback.Artist,
		Suffix:      strings.TrimPrefix(extension, "."),
		ContentType: contentType(extension),
		Size:        info.Size(),
	}

	file, err := os.Open(path)
	if err != nil {
		return scanner.Metadata{}, fmt.Errorf("open: %w", err)
	}
	defer file.Close()
	values, err := tag.ReadFrom(file)
	if err != nil {
		if errors.Is(err, tag.ErrNoTagsFound) {
			return result, nil
		}
		return scanner.Metadata{}, fmt.Errorf("read tags: %w", err)
	}
	if value := strings.TrimSpace(values.Title()); value != "" {
		result.Title = value
	}
	if value := strings.TrimSpace(values.Artist()); value != "" {
		result.Artist = value
	}
	if value := strings.TrimSpace(values.Album()); value != "" {
		result.Album = value
	}
	if value := strings.TrimSpace(values.AlbumArtist()); value != "" {
		result.AlbumArtist = value
	} else {
		result.AlbumArtist = result.Artist
	}
	result.TrackNumber, _ = values.Track()
	result.DiscNumber, _ = values.Disc()
	result.Year = values.Year()
	return result, nil
}

func fallbackMetadata(path string) scanner.Metadata {
	albumDir := filepath.Dir(path)
	artistDir := filepath.Dir(albumDir)
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	album := filepath.Base(albumDir)
	artist := filepath.Base(artistDir)
	if album == "." || album == string(filepath.Separator) || album == "" {
		album = "Unknown Album"
	}
	if artist == "." || artist == string(filepath.Separator) || artist == "" {
		artist = "Unknown Artist"
	}
	return scanner.Metadata{Title: title, Artist: artist, Album: album}
}

func contentType(extension string) string {
	types := map[string]string{
		".mp3":  "audio/mpeg",
		".flac": "audio/flac",
		".ogg":  "audio/ogg",
		".oga":  "audio/ogg",
		".opus": "audio/ogg",
		".m4a":  "audio/mp4",
		".aac":  "audio/aac",
		".wav":  "audio/wav",
	}
	if value := types[extension]; value != "" {
		return value
	}
	if value := mime.TypeByExtension(extension); value != "" {
		return value
	}
	return "application/octet-stream"
}
