package torrent

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const Name = "torrent"

var audioContentTypes = map[string]string{
	".aac": "audio/aac", ".flac": "audio/flac", ".m4a": "audio/mp4",
	".mp3": "audio/mpeg", ".oga": "audio/ogg", ".ogg": "audio/ogg",
	".opus": "audio/ogg", ".wav": "audio/wav",
}

type Provider struct{}

type Catalog struct {
	InfoHash string
	Name     string
	Tracks   []domain.TrackSource
}

func New() *Provider { return &Provider{} }

func (*Provider) Name() string { return Name }

func (*Provider) Search(context.Context, domain.SearchQuery) ([]domain.TrackSource, error) {
	return nil, nil
}

func (*Provider) Resolve(_ context.Context, ref domain.SourceRef) (ports.ResolvedSource, error) {
	if ref.Provider != Name {
		return ports.ResolvedSource{}, fmt.Errorf("cannot resolve provider %q", ref.Provider)
	}
	return ports.ResolvedSource{}, fmt.Errorf("%w: torrent media has not been downloaded", ports.ErrSourceUnavailable)
}

func (*Provider) ReadCatalog(reader io.Reader) (Catalog, error) {
	meta, err := metainfo.Load(reader)
	if err != nil {
		return Catalog{}, fmt.Errorf("read torrent metainfo: %w", err)
	}
	info, err := meta.UnmarshalInfo()
	if err != nil {
		return Catalog{}, fmt.Errorf("decode torrent info: %w", err)
	}
	name := strings.TrimSpace(info.BestName())
	if name == "" {
		return Catalog{}, fmt.Errorf("torrent name is empty")
	}
	infoHash := meta.HashInfoBytes().HexString()
	result := Catalog{InfoHash: infoHash, Name: name}
	for _, file := range info.UpvertedFiles() {
		parts, err := safePath(name, file.BestPath())
		if err != nil {
			return Catalog{}, err
		}
		extension := strings.ToLower(path.Ext(parts[len(parts)-1]))
		contentType, supported := audioContentTypes[extension]
		if !supported {
			continue
		}
		if detected := mime.TypeByExtension(extension); detected != "" {
			contentType = detected
		}
		result.Tracks = append(result.Tracks, makeTrack(infoHash, parts, file.Length, extension, contentType))
	}
	return result, nil
}

func safePath(root string, fileParts []string) ([]string, error) {
	parts := make([]string, 0, len(fileParts)+1)
	parts = append(parts, root)
	for _, part := range fileParts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `/\\`) {
			return nil, fmt.Errorf("torrent contains unsafe path component %q", part)
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func makeTrack(infoHash string, parts []string, size int64, extension, contentType string) domain.TrackSource {
	fileName := parts[len(parts)-1]
	title, trackNumber := titleAndNumber(strings.TrimSuffix(fileName, path.Ext(fileName)))
	if len(parts) == 1 {
		if _, candidate, ok := strings.Cut(title, " - "); ok && strings.TrimSpace(candidate) != "" {
			title = strings.TrimSpace(candidate)
		}
	}
	artist, album := provisionalArtistAlbum(parts)
	artistID := domain.StableID("artist", strings.ToLower(artist))
	albumID := domain.StableID("album", artistID, strings.ToLower(album))
	logicalPath := strings.Join(parts, "/")
	return domain.TrackSource{
		Track: domain.Track{
			ID: domain.StableID("track", Name, infoHash, logicalPath), Title: title,
			Artist: artist, ArtistID: artistID, Album: album, AlbumID: albumID,
			AlbumArtist: artist, AlbumArtistID: artistID, TrackNumber: trackNumber,
			Size: size, Suffix: strings.TrimPrefix(extension, "."), ContentType: contentType,
		},
		Ref: domain.SourceRef{Provider: Name, Key: infoHash + "/" + logicalPath},
	}
}

func provisionalArtistAlbum(parts []string) (artist, album string) {
	root := parts[0]
	if len(parts) == 1 {
		root = strings.TrimSuffix(root, path.Ext(root))
	}
	rootArtist, rootAlbum, rootSplit := strings.Cut(root, " - ")
	directories := parts[:len(parts)-1]
	switch {
	case len(directories) >= 3:
		artist, album = directories[len(directories)-2], directories[len(directories)-1]
	case len(parts) == 1 && rootSplit:
		artist, album = strings.TrimSpace(rootArtist), "Unknown Album"
	case rootSplit:
		artist, album = strings.TrimSpace(rootArtist), strings.TrimSpace(rootAlbum)
	case len(directories) == 2:
		artist, album = directories[0], directories[1]
	default:
		artist, album = "Unknown Artist", root
	}
	if artist == "" {
		artist = "Unknown Artist"
	}
	if album == "" {
		album = root
	}
	return artist, album
}

func titleAndNumber(value string) (string, int) {
	value = strings.TrimSpace(value)
	index := 0
	for index < len(value) && index < 3 && value[index] >= '0' && value[index] <= '9' {
		index++
	}
	if index == 0 || index == len(value) || !isTrackSeparator(rune(value[index])) {
		return value, 0
	}
	number, err := strconv.Atoi(value[:index])
	if err != nil {
		return value, 0
	}
	title := strings.TrimLeftFunc(value[index:], isTrackSeparator)
	if title == "" {
		return value, 0
	}
	return title, number
}

func isTrackSeparator(value rune) bool {
	return unicode.IsSpace(value) || value == '.' || value == '-' || value == '_'
}
