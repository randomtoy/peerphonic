package scanner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const LocalProvider = "local"

var supportedExtensions = map[string]struct{}{
	".mp3": {}, ".flac": {}, ".ogg": {}, ".oga": {}, ".opus": {},
	".m4a": {}, ".aac": {}, ".wav": {},
}

type Metadata struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	TrackNumber int
	DiscNumber  int
	Year        int
	Duration    time.Duration
	Size        int64
	BitRate     int
	Suffix      string
	ContentType string
	Artwork     *Artwork
}

type Artwork struct {
	Data []byte
}

type Extractor interface {
	Extract(path string, info os.FileInfo) (Metadata, error)
}

type Warning struct {
	Path string
	Err  error
}

type Report struct {
	Tracks   int
	Warnings []Warning
}

type Scanner struct {
	root      string
	catalog   ports.Catalog
	extractor Extractor
	artwork   ArtworkWriter
}

type ArtworkWriter interface {
	Put(ctx context.Context, data []byte) (string, error)
}

func New(root string, catalog ports.Catalog, extractor Extractor, artwork ...ArtworkWriter) *Scanner {
	result := &Scanner{root: root, catalog: catalog, extractor: extractor}
	if len(artwork) > 0 {
		result.artwork = artwork[0]
	}
	return result
}

func (s *Scanner) Scan(ctx context.Context) (Report, error) {
	root, err := filepath.Abs(s.root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve music directory: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Report{}, fmt.Errorf("stat music directory: %w", err)
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("music path %q is not a directory", root)
	}

	var tracks []domain.Track
	var warnings []Warning
	artworkIDs := make(map[[sha256.Size]byte]string)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			warnings = append(warnings, Warning{Path: path, Err: walkErr})
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !isSupported(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			warnings = append(warnings, Warning{Path: path, Err: err})
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		metadata, err := s.extractor.Extract(path, info)
		if err != nil {
			warnings = append(warnings, Warning{Path: path, Err: err})
			return nil
		}
		key, err := filepath.Rel(root, path)
		if err != nil {
			warnings = append(warnings, Warning{Path: path, Err: err})
			return nil
		}
		track := makeTrack(filepath.ToSlash(key), metadata)
		if metadata.Artwork != nil && s.artwork != nil {
			digest := sha256.Sum256(metadata.Artwork.Data)
			if coverArtID, ok := artworkIDs[digest]; ok {
				track.CoverArtID = coverArtID
			} else {
				coverArtID, err := s.artwork.Put(ctx, metadata.Artwork.Data)
				if err != nil {
					warnings = append(warnings, Warning{Path: path, Err: fmt.Errorf("store artwork: %w", err)})
				} else {
					artworkIDs[digest] = coverArtID
					track.CoverArtID = coverArtID
				}
			}
		}
		tracks = append(tracks, track)
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("walk music directory: %w", err)
	}
	if err := s.catalog.ReplaceProviderTracks(ctx, LocalProvider, tracks); err != nil {
		return Report{}, fmt.Errorf("replace local catalog: %w", err)
	}
	return Report{Tracks: len(tracks), Warnings: warnings}, nil
}

func makeTrack(key string, metadata Metadata) domain.Track {
	artistName := strings.TrimSpace(metadata.AlbumArtist)
	if artistName == "" {
		artistName = strings.TrimSpace(metadata.Artist)
	}
	if artistName == "" {
		artistName = "Unknown Artist"
	}
	trackArtist := strings.TrimSpace(metadata.Artist)
	if trackArtist == "" {
		trackArtist = artistName
	}
	albumName := strings.TrimSpace(metadata.Album)
	if albumName == "" {
		albumName = "Unknown Album"
	}
	title := strings.TrimSpace(metadata.Title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(key), filepath.Ext(key))
	}
	artistID := domain.StableID("artist", strings.ToLower(artistName))
	albumID := domain.StableID("album", artistID, strings.ToLower(albumName))
	return domain.Track{
		ID:          domain.StableID("track", LocalProvider, key),
		Title:       title,
		Artist:      trackArtist,
		ArtistID:    artistID,
		Album:       albumName,
		AlbumID:     albumID,
		AlbumArtist: artistName,
		Source:      domain.SourceRef{Provider: LocalProvider, Key: key},
		TrackNumber: metadata.TrackNumber,
		DiscNumber:  metadata.DiscNumber,
		Year:        metadata.Year,
		Duration:    metadata.Duration,
		Size:        metadata.Size,
		BitRate:     metadata.BitRate,
		Suffix:      metadata.Suffix,
		ContentType: metadata.ContentType,
	}
}

func isSupported(path string) bool {
	_, ok := supportedExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}
