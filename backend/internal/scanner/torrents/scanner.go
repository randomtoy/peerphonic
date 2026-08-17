package torrents

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

type Scanner struct {
	root      string
	catalog   ports.Catalog
	provider  *torrentprovider.Provider
	extractor metadataExtractor
	artwork   scanner.ArtworkWriter
}

func New(root string, catalog ports.Catalog, provider *torrentprovider.Provider) *Scanner {
	return &Scanner{root: root, catalog: catalog, provider: provider}
}

func NewWithEnrichment(
	root string,
	catalog ports.Catalog,
	provider *torrentprovider.Provider,
	extractor metadataExtractor,
	artwork scanner.ArtworkWriter,
) *Scanner {
	return &Scanner{
		root: root, catalog: catalog, provider: provider, extractor: extractor, artwork: artwork,
	}
}

func (s *Scanner) Scan(ctx context.Context) (scanner.Report, error) {
	var tracks []domain.TrackSource
	var warnings []scanner.Warning
	seen := make(map[string]struct{})
	err := filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if os.IsNotExist(walkErr) && path == s.root {
				return fs.SkipAll
			}
			warnings = append(warnings, scanner.Warning{Path: path, Err: walkErr})
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".torrent") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			warnings = append(warnings, scanner.Warning{Path: path, Err: err})
			return nil
		}
		metadata, parseErr := s.provider.ReadCatalog(file)
		closeErr := file.Close()
		if parseErr != nil {
			warnings = append(warnings, scanner.Warning{Path: path, Err: parseErr})
			return nil
		}
		if closeErr != nil {
			warnings = append(warnings, scanner.Warning{Path: path, Err: closeErr})
		}
		for _, track := range metadata.Tracks {
			if _, exists := seen[track.Ref.Key]; exists {
				continue
			}
			seen[track.Ref.Key] = struct{}{}
			if s.extractor != nil {
				if cachedPath, ok := s.provider.CachedPath(track.Ref, track.Track.Size); ok {
					if err := enrichTrack(ctx, &track.Track, cachedPath, s.extractor, s.artwork); err != nil {
						warnings = append(warnings, scanner.Warning{Path: cachedPath, Err: err})
					}
				}
			}
			tracks = append(tracks, track)
		}
		return nil
	})
	if err != nil {
		return scanner.Report{}, fmt.Errorf("walk torrent metadata directory: %w", err)
	}
	if err := s.catalog.ReplaceProviderTracks(ctx, torrentprovider.Name, tracks, nil); err != nil {
		return scanner.Report{}, fmt.Errorf("replace torrent catalog: %w", err)
	}
	return scanner.Report{Tracks: len(tracks), Warnings: warnings}, nil
}
