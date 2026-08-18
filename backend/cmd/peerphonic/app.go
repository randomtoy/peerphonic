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
	handler          http.Handler
	catalog          *sqlite.Catalog
	torrentProvider  *torrentprovider.Provider
	soulseekProvider *soulseekprovider.Client
	magnetImporter   *torrentscanner.MagnetImporter
}

func buildApplication(ctx context.Context, cfg config.Config, logger *slog.Logger) (*application, error) {
	catalog, err := sqlite.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	var torrentProvider *torrentprovider.Provider
	var soulseekClient *soulseekprovider.Client
	var magnetImporter *torrentscanner.MagnetImporter
	fail := func(err error) (*application, error) {
		if magnetImporter != nil {
			magnetImporter.Close()
		}
		if torrentProvider != nil {
			_ = torrentProvider.Close()
		}
		if soulseekClient != nil {
			_ = soulseekClient.Close()
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
	if cfg.SlskdURL != "" {
		var clientErr error
		soulseekClient, clientErr = soulseekprovider.NewSlskd(
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
	}
	blobs, err := filesystem.New(cfg.CacheDir)
	if err != nil {
		return fail(err)
	}
	artworkSources := []services.ArtworkSource{torrentProvider}
	if soulseekClient != nil {
		artworkSources = append(artworkSources, soulseekClient)
	}
	artwork := services.NewArtworkService(blobs, artworkSources...)
	completedEnricher := scanner.NewEnricher(catalog, metadata.TagExtractor{}, artwork)
	torrentProvider.SetCompletedHandler(func(file torrentprovider.CompletedFile) {
		if err := completedEnricher.Enrich(ctx, file.TrackID, file.Path); err != nil {
			logger.Warn("torrent metadata enrichment failed", "track", file.TrackID, "error", err)
			return
		}
		logger.Info("torrent metadata enriched", "track", file.TrackID)
	})
	if soulseekClient != nil {
		soulseekClient.SetCompletedHandler(func(file soulseekprovider.CompletedFile) {
			if err := completedEnricher.Enrich(context.WithoutCancel(ctx), file.TrackID, file.Path); err != nil {
				logger.Warn("Soulseek metadata enrichment failed", "track", file.TrackID, "error", err)
				return
			}
			logger.Info("Soulseek metadata enriched", "track", file.TrackID)
		})
	}
	mediaCache, err := services.NewMediaCache(blobs, catalog, cfg.CacheSizeBytes)
	if err != nil {
		return fail(err)
	}
	if err := mediaCache.Prune(ctx); err != nil {
		return fail(fmt.Errorf("prune media cache: %w", err))
	}
	cacheStatus := services.NewCacheStatus(mediaCache, torrentProvider)
	if soulseekClient != nil {
		cacheStatus = services.NewCacheStatus(mediaCache, torrentProvider, soulseekClient)
	}
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
	if soulseekClient != nil {
		if err := soulseekClient.ResumeTrackDownloads(ctx); err != nil {
			logger.Warn("some Soulseek track downloads could not be resumed", "error", err)
		}
	}

	streamingProviders := []ports.SourceProvider{provider, torrentProvider}
	downloadMonitors := []ports.TrackDownloadMonitor{torrentProvider}
	if soulseekClient != nil {
		streamingProviders = append(streamingProviders, soulseekClient)
		downloadMonitors = append(downloadMonitors, soulseekClient)
	}
	streaming := services.NewStreamingService(catalog, streamingProviders...)
	downloadService := services.NewDownloadService(downloadMonitors...)
	mux := http.NewServeMux()
	mux.Handle("/rest/", opensubsonic.NewHandlerWithAuthenticator(
		catalog, streaming, artwork, userService, scanManager,
	))
	mux.Handle("/", peerphonic.NewHandlerWithAuthenticator(
		cacheStatus, torrentImporter, magnetImporter, torrentManager, torrentProvider, downloadService,
		userService, userService, soulseekMonitor, soulseekSearch, transferSettings, scanManager,
	))
	return &application{
		handler: mux, catalog: catalog, torrentProvider: torrentProvider,
		soulseekProvider: soulseekClient, magnetImporter: magnetImporter,
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
	if a.soulseekProvider != nil {
		if err := a.soulseekProvider.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close Soulseek provider: %w", err))
		}
	}
	if err := a.catalog.Close(); err != nil {
		closeErrors = append(closeErrors, fmt.Errorf("close catalog: %w", err))
	}
	return errors.Join(closeErrors...)
}
