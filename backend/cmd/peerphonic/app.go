package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/blob/filesystem"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/metadata"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/providers/local"
	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/api/opensubsonic"
	"github.com/randomtoy/peerphonic/backend/internal/api/peerphonic"
	"github.com/randomtoy/peerphonic/backend/internal/config"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
	torrentscanner "github.com/randomtoy/peerphonic/backend/internal/scanner/torrents"
)

type application struct {
	handler http.Handler
	catalog *sqlite.Catalog
}

func buildApplication(ctx context.Context, cfg config.Config, logger *slog.Logger) (*application, error) {
	catalog, err := sqlite.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*application, error) {
		catalog.Close()
		return nil, err
	}

	provider, err := local.New(cfg.MusicDir)
	if err != nil {
		return fail(err)
	}
	torrentProvider := torrentprovider.New()
	blobs, err := filesystem.New(cfg.CacheDir)
	if err != nil {
		return fail(err)
	}
	artwork := services.NewArtworkService(blobs)
	mediaCache, err := services.NewMediaCache(blobs, catalog, cfg.CacheSizeBytes)
	if err != nil {
		return fail(err)
	}
	if err := mediaCache.Prune(ctx); err != nil {
		return fail(fmt.Errorf("prune media cache: %w", err))
	}
	localScanner := scanner.New(cfg.MusicDir, catalog, metadata.TagExtractor{}, artwork)
	torrentScanner := torrentscanner.New(cfg.TorrentDir, catalog, torrentProvider)
	scanManager := scanner.NewManager(ctx, scanner.NewGroup(localScanner, torrentScanner))
	torrentImporter := torrentscanner.NewImporter(cfg.TorrentDir, torrentProvider, scanManager)
	if cfg.Scan {
		report, err := scanManager.ScanNow(ctx)
		if err != nil {
			return fail(err)
		}
		logger.Info("music scan completed", "tracks", report.Tracks, "warnings", len(report.Warnings))
		for _, warning := range report.Warnings {
			logger.Warn("music scan warning", "path", warning.Path, "error", warning.Err)
		}
	}

	streaming := services.NewStreamingService(catalog, provider, torrentProvider)
	mux := http.NewServeMux()
	mux.Handle("/rest/", opensubsonic.NewHandler(catalog, streaming, artwork, cfg.Username, cfg.Password, scanManager))
	mux.Handle("/", peerphonic.NewHandler(mediaCache, torrentImporter, cfg.Username, cfg.Password))
	return &application{handler: mux, catalog: catalog}, nil
}

func (a *application) Close() error {
	if err := a.catalog.Close(); err != nil {
		return fmt.Errorf("close catalog: %w", err)
	}
	return nil
}
