package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	authadapter "github.com/randomtoy/peerphonic/backend/internal/adapters/auth"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/blob/filesystem"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/metadata"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/providers/local"
	soulseekprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/soulseek"
	torrentprovider "github.com/randomtoy/peerphonic/backend/internal/adapters/providers/torrent"
	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/api/opensubsonic"
	"github.com/randomtoy/peerphonic/backend/internal/api/peerphonic"
	"github.com/randomtoy/peerphonic/backend/internal/config"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
	torrentscanner "github.com/randomtoy/peerphonic/backend/internal/scanner/torrents"
)

type application struct {
	handler         http.Handler
	catalog         *sqlite.Catalog
	torrentProvider *torrentprovider.Provider
	magnetImporter  *torrentscanner.MagnetImporter
}

func buildApplication(ctx context.Context, cfg config.Config, logger *slog.Logger) (*application, error) {
	catalog, err := sqlite.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	var torrentProvider *torrentprovider.Provider
	var magnetImporter *torrentscanner.MagnetImporter
	fail := func(err error) (*application, error) {
		if magnetImporter != nil {
			magnetImporter.Close()
		}
		if torrentProvider != nil {
			_ = torrentProvider.Close()
		}
		catalog.Close()
		return nil, err
	}
	credentialCodec, err := authadapter.NewCredentialCodec(cfg.Database + ".auth.key")
	if err != nil {
		return fail(fmt.Errorf("initialize user credentials: %w", err))
	}
	userService := services.NewUserService(catalog, credentialCodec)
	if err := userService.EnsureBootstrapAdmin(ctx, cfg.Username, cfg.Password); err != nil {
		return fail(fmt.Errorf("initialize bootstrap administrator: %w", err))
	}

	provider, err := local.New(cfg.MusicDir)
	if err != nil {
		return fail(err)
	}
	torrentProvider, err = torrentprovider.NewStreaming(
		cfg.TorrentDir, filepath.Join(cfg.CacheDir, "torrents"),
		torrentprovider.StreamingOptions{
			Seed:           cfg.TorrentSeed,
			ListenPort:     cfg.TorrentPort,
			PortForwarding: cfg.TorrentPortForwarding,
			UploadLimit:    cfg.TorrentUploadLimit,
			DownloadLimit:  cfg.TorrentDownloadLimit,
		},
	)
	if err != nil {
		return fail(err)
	}
	transferSettings := services.NewTransferSettingsService(catalog, torrentProvider)
	if err := transferSettings.Initialize(ctx); err != nil {
		return fail(fmt.Errorf("initialize transfer settings: %w", err))
	}
	var soulseekMonitor ports.ProviderStatusMonitor
	var soulseekSearch ports.SourceSearcher
	var soulseekStreaming ports.SourceProvider
	if cfg.SlskdURL != "" {
		soulseekClient, clientErr := soulseekprovider.NewSlskd(
			cfg.SlskdURL, cfg.SlskdAPIKey, time.Duration(cfg.SlskdTimeoutSeconds)*time.Second,
		)
		if clientErr != nil {
			return fail(fmt.Errorf("initialize slskd client: %w", clientErr))
		}
		if clientErr := soulseekClient.SetMediaDirectories(
			cfg.SlskdDownloadsDir, cfg.SlskdIncompleteDir,
		); clientErr != nil {
			return fail(fmt.Errorf("initialize slskd media directories: %w", clientErr))
		}
		soulseekMonitor = soulseekClient
		soulseekSearch = services.NewDiscoveryService(soulseekClient, catalog)
		soulseekStreaming = soulseekClient
	}
	blobs, err := filesystem.New(cfg.CacheDir)
	if err != nil {
		return fail(err)
	}
	artwork := services.NewArtworkService(blobs, torrentProvider)
	torrentEnricher := torrentscanner.NewEnricher(catalog, metadata.TagExtractor{}, artwork)
	torrentProvider.SetCompletedHandler(func(file torrentprovider.CompletedFile) {
		if err := torrentEnricher.Enrich(ctx, file.TrackID, file.Path); err != nil {
			logger.Warn("torrent metadata enrichment failed", "track", file.TrackID, "error", err)
			return
		}
		logger.Info("torrent metadata enriched", "track", file.TrackID)
	})
	mediaCache, err := services.NewMediaCache(blobs, catalog, cfg.CacheSizeBytes)
	if err != nil {
		return fail(err)
	}
	if err := mediaCache.Prune(ctx); err != nil {
		return fail(fmt.Errorf("prune media cache: %w", err))
	}
	cacheStatus := services.NewCacheStatus(mediaCache, torrentProvider)
	torrentProvider.SetCacheChangedHandler(func() {
		if err := cacheStatus.Prune(context.WithoutCancel(ctx)); err != nil {
			logger.Warn("media cache pruning failed", "error", err)
		}
	})
	localScanner := scanner.New(cfg.MusicDir, catalog, metadata.TagExtractor{}, artwork)
	torrentScanner := torrentscanner.NewWithEnrichment(
		cfg.TorrentDir, catalog, torrentProvider, metadata.TagExtractor{}, artwork,
	)
	scanManager := scanner.NewManager(ctx, scanner.NewGroup(localScanner, torrentScanner))
	torrentImporter := torrentscanner.NewImporter(cfg.TorrentDir, torrentProvider, scanManager)
	torrentManager := torrentscanner.NewSourceManager(torrentProvider, scanManager)
	if cfg.Scan {
		report, err := scanManager.ScanNow(ctx)
		if err != nil {
			return fail(err)
		}
		logger.Info("music scan completed", "tracks", report.Tracks, "warnings", len(report.Warnings))
		for _, warning := range report.Warnings {
			logger.Warn("music scan warning", "path", warning.Path, "error", warning.Err)
		}
		if err := cacheStatus.Prune(ctx); err != nil {
			return fail(fmt.Errorf("prune provider cache: %w", err))
		}
	}
	magnetImporter, err = torrentscanner.NewMagnetImporter(ctx, cfg.TorrentDir, torrentProvider, torrentImporter)
	if err != nil {
		return fail(fmt.Errorf("initialize magnet imports: %w", err))
	}
	scanManager.StartPeriodic(time.Duration(cfg.ScanIntervalSeconds) * time.Second)
	if err := torrentProvider.ResumeTrackDownloads(ctx); err != nil {
		logger.Warn("some torrent track downloads could not be resumed", "error", err)
	}

	streamingProviders := []ports.SourceProvider{provider, torrentProvider}
	if soulseekStreaming != nil {
		streamingProviders = append(streamingProviders, soulseekStreaming)
	}
	streaming := services.NewStreamingService(catalog, streamingProviders...)
	mux := http.NewServeMux()
	mux.Handle("/rest/", opensubsonic.NewHandlerWithAuthenticator(
		catalog, streaming, artwork, userService, scanManager,
	))
	mux.Handle("/", peerphonic.NewHandlerWithAuthenticator(
		cacheStatus, torrentImporter, magnetImporter, torrentManager, torrentProvider, torrentProvider,
		userService, userService, soulseekMonitor, soulseekSearch, transferSettings, scanManager,
	))
	return &application{
		handler: mux, catalog: catalog, torrentProvider: torrentProvider, magnetImporter: magnetImporter,
	}, nil
}

func (a *application) Close() error {
	var closeErrors []error
	if a.magnetImporter != nil {
		a.magnetImporter.Close()
	}
	if a.torrentProvider != nil {
		if err := a.torrentProvider.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close torrent provider: %w", err))
		}
	}
	if err := a.catalog.Close(); err != nil {
		closeErrors = append(closeErrors, fmt.Errorf("close catalog: %w", err))
	}
	return errors.Join(closeErrors...)
}
