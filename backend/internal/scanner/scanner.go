package scanner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/audioformat"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const LocalProvider = "local"

type Metadata struct {
	Title               string
	Artist              string
	Album               string
	AlbumArtist         string
	AlbumArtistExplicit bool
	TrackNumber         int
	DiscNumber          int
	Year                int
	Genre               string
	Duration            time.Duration
	Size                int64
	BitRate             int
	Suffix              string
	ContentType         string
	Artwork             *Artwork
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

	var tracks []domain.TrackSource
	var warnings []Warning
	artworkIDs := make(map[[sha256.Size]byte]string)
	explicitAlbumArtists := make(map[string]bool)
	originalAlbumIDs := make(map[string]string)
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
		track.DiscoveredAt = info.ModTime().UTC()
		explicitAlbumArtists[track.Track.ID] = metadata.AlbumArtistExplicit
		originalAlbumIDs[track.Track.ID] = track.Track.AlbumID
		if metadata.Artwork != nil && s.artwork != nil {
			digest := sha256.Sum256(metadata.Artwork.Data)
			if coverArtID, ok := artworkIDs[digest]; ok {
				track.Track.CoverArtID = coverArtID
			} else {
				coverArtID, err := s.artwork.Put(ctx, metadata.Artwork.Data)
				if err != nil {
					warnings = append(warnings, Warning{Path: path, Err: fmt.Errorf("store artwork: %w", err)})
				} else {
					artworkIDs[digest] = coverArtID
					track.Track.CoverArtID = coverArtID
				}
			}
		}
		tracks = append(tracks, track)
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("walk music directory: %w", err)
	}
	normalizeCompilationAlbums(tracks, explicitAlbumArtists)
	aliases := changedAlbumAliases(tracks, originalAlbumIDs)
	if err := s.catalog.ReplaceProviderTracks(ctx, LocalProvider, tracks, aliases); err != nil {
		return Report{}, fmt.Errorf("replace local catalog: %w", err)
	}
	return Report{Tracks: len(tracks), Warnings: warnings}, nil
}

func changedAlbumAliases(tracks []domain.TrackSource, originalAlbumIDs map[string]string) []ports.AlbumAlias {
	unique := make(map[ports.AlbumAlias]struct{})
	for _, source := range tracks {
		originalID := originalAlbumIDs[source.Track.ID]
		if originalID != "" && originalID != source.Track.AlbumID {
			unique[ports.AlbumAlias{AliasID: originalID, TrackID: source.Track.ID}] = struct{}{}
		}
	}
	aliases := make([]ports.AlbumAlias, 0, len(unique))
	for alias := range unique {
		aliases = append(aliases, alias)
	}
	return aliases
}

func makeTrack(key string, metadata Metadata) domain.TrackSource {
	track := domain.Track{
		ID:    domain.StableID("track", LocalProvider, key),
		Title: strings.TrimSuffix(filepath.Base(key), filepath.Ext(key)),
	}
	ApplyMetadata(&track, metadata)
	return domain.TrackSource{
		Track: track,
		Ref:   domain.SourceRef{Provider: LocalProvider, Key: key},
	}
}

// ApplyMetadata enriches an existing logical track without changing its stable
// identity or source references.
func ApplyMetadata(track *domain.Track, metadata Metadata) {
	albumArtistName := strings.TrimSpace(metadata.AlbumArtist)
	if albumArtistName == "" {
		albumArtistName = strings.TrimSpace(metadata.Artist)
	}
	if albumArtistName == "" {
		albumArtistName = "Unknown Artist"
	}
	trackArtist := strings.TrimSpace(metadata.Artist)
	if trackArtist == "" {
		trackArtist = albumArtistName
	}
	albumName := strings.TrimSpace(metadata.Album)
	if albumName == "" {
		albumName = "Unknown Album"
	}
	title := strings.TrimSpace(metadata.Title)
	if title == "" {
		title = track.Title
	}
	trackArtistID := domain.CanonicalArtistID(trackArtist)
	albumArtistID := domain.CanonicalArtistID(albumArtistName)
	albumID := domain.CanonicalAlbumID(albumArtistName, albumName)
	track.Title = title
	track.Artist = trackArtist
	track.ArtistID = trackArtistID
	track.Album = albumName
	track.AlbumID = albumID
	track.AlbumArtist = albumArtistName
	track.AlbumArtistID = albumArtistID
	track.TrackNumber = metadata.TrackNumber
	track.DiscNumber = metadata.DiscNumber
	track.Year = metadata.Year
	track.Genre = strings.TrimSpace(metadata.Genre)
	track.Duration = metadata.Duration
	track.Size = metadata.Size
	track.BitRate = metadata.BitRate
	track.Suffix = metadata.Suffix
	track.ContentType = metadata.ContentType
}

func normalizeCompilationAlbums(tracks []domain.TrackSource, explicitAlbumArtists map[string]bool) {
	groups := make(map[string][]int)
	for index, source := range tracks {
		directory := path.Dir(source.Ref.Key)
		if directory != "." && directory != "/" {
			groups[directory] = append(groups[directory], index)
		}
	}
	for directory, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		artists := make(map[string]struct{})
		albumArtists := make(map[string]struct{})
		hasExplicitAlbumArtist := false
		for _, index := range indexes {
			track := tracks[index].Track
			artists[strings.ToLower(strings.TrimSpace(track.Artist))] = struct{}{}
			albumArtists[strings.ToLower(strings.TrimSpace(track.AlbumArtist))] = struct{}{}
			if explicitAlbumArtists[track.ID] {
				hasExplicitAlbumArtist = true
			}
		}
		if len(artists) < 2 {
			continue
		}
		oneAlbumArtistIsVarious := false
		if len(albumArtists) == 1 {
			for albumArtist := range albumArtists {
				oneAlbumArtistIsVarious = isVariousArtistName(albumArtist)
			}
		}
		isCompilation := len(albumArtists) > 1 || !hasExplicitAlbumArtist ||
			(len(albumArtists) == 1 && oneAlbumArtistIsVarious)
		if !isCompilation {
			continue
		}
		albumName := compilationAlbumName(tracks, indexes, directory)
		albumArtist := "Various Artists"
		albumArtistID := domain.CanonicalArtistID(albumArtist)
		albumID := domain.CanonicalAlbumID(albumArtist, albumName)
		for _, index := range indexes {
			tracks[index].Track.Album = albumName
			tracks[index].Track.AlbumArtist = albumArtist
			tracks[index].Track.AlbumArtistID = albumArtistID
			tracks[index].Track.AlbumID = albumID
		}
	}
}

func compilationAlbumName(tracks []domain.TrackSource, indexes []int, directory string) string {
	names := make(map[string]string)
	for _, index := range indexes {
		name := strings.TrimSpace(tracks[index].Track.Album)
		if name != "" {
			names[strings.ToLower(name)] = name
		}
	}
	if len(names) == 1 {
		for folded, name := range names {
			if !isGenericCompilationAlbumName(folded) {
				return name
			}
		}
	}
	return path.Base(directory)
}

func isGenericCompilationAlbumName(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "various artists", "various", "va", "v/a", "unknown album":
		return true
	default:
		return false
	}
}

func isVariousArtistName(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "various artists", "various", "va", "v/a":
		return true
	default:
		return false
	}
}

func isSupported(path string) bool {
	_, ok := audioformat.FromPath(path)
	return ok
}
