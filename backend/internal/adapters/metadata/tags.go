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
	if err != nil && !errors.Is(err, tag.ErrNoTagsFound) {
		return scanner.Metadata{}, fmt.Errorf("read tags: %w", err)
	}
	if err == nil {
		if value, ok := usableMetadataText(values.Title()); ok {
			result.Title = value
		}
		if value, ok := usableMetadataText(values.Artist()); ok {
			result.Artist = value
		}
		if value, ok := usableMetadataText(values.Album()); ok {
			result.Album = value
		}
		if value, ok := usableMetadataText(values.AlbumArtist()); ok {
			result.AlbumArtist = value
			result.AlbumArtistExplicit = true
		} else {
			result.AlbumArtist = result.Artist
		}
		result.TrackNumber, _ = values.Track()
		result.DiscNumber, _ = values.Disc()
		result.Year = values.Year()
		if value, ok := usableMetadataText(values.Genre()); ok {
			result.Genre = value
		}
	}
	result.Artwork = extractArtwork(path, values)
	if err := populateAudioProperties(file, extension, &result); err != nil {
		return scanner.Metadata{}, err
	}
	return result, nil
}

func fallbackMetadata(path string) scanner.Metadata {
	albumDir := filepath.Dir(path)
	artistDir := filepath.Dir(albumDir)
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	album := filepath.Base(albumDir)
	artist := filepath.Base(artistDir)
	if filenameArtist, filenameTitle, matched := artistAndTitleFromFilename(title); matched {
		title = filenameTitle
		if filenameArtist != "" {
			artist = filenameArtist
		} else {
			artist = "Unknown Artist"
		}
	}
	if album == "." || album == string(filepath.Separator) || album == "" {
		album = "Unknown Album"
	}
	if artist == "." || artist == string(filepath.Separator) || artist == "" {
		artist = "Unknown Artist"
	}
	return scanner.Metadata{Title: title, Artist: artist, Album: album}
}

func artistAndTitleFromFilename(filename string) (string, string, bool) {
	withoutTrack := strings.TrimLeftFunc(filename, func(value rune) bool {
		return value >= '0' && value <= '9'
	})
	if withoutTrack == filename {
		return "", "", false
	}
	withoutTrack = strings.TrimSpace(withoutTrack)
	if withoutTrack != "" && strings.ContainsRune("._-", rune(withoutTrack[0])) {
		withoutTrack = strings.TrimSpace(withoutTrack[1:])
	}
	artist, title, ok := strings.Cut(withoutTrack, " - ")
	if !ok {
		return "", "", false
	}
	artist, _ = usableMetadataText(artist)
	title, titleOK := usableMetadataText(title)
	if !titleOK {
		return "", "", false
	}
	return artist, title, true
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
