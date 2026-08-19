package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
	"github.com/randomtoy/peerphonic/backend/internal/backup"
	"github.com/randomtoy/peerphonic/backend/internal/config"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("peerphonic stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("usage: peerphonic <serve|backup|restore>")
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:], logger)
	case "backup":
		return runBackup(args[1:], logger)
	case "restore":
		return runRestore(args[1:], logger)
	default:
		return fmt.Errorf("unknown command %q (use serve, backup, or restore)", args[0])
	}
}

func runRestore(args []string, logger *slog.Logger) error {
	databasePath := "peerphonic.db"
	if configured, ok := os.LookupEnv("PEERPHONIC_DATABASE"); ok {
		databasePath = configured
	}
	var inputPath, credentialKeyPath string
	var force bool
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&inputPath, "input", "", "backup archive path")
	flags.StringVar(&databasePath, "database", databasePath, "SQLite database path")
	flags.StringVar(&credentialKeyPath, "credential-key", "", "credential encryption key path")
	flags.BoolVar(&force, "force", false, "replace existing files after preserving recovery copies")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || inputPath == "" {
		return errors.New("restore requires --input and accepts no positional arguments")
	}
	var err error
	if inputPath, err = filepath.Abs(inputPath); err != nil {
		return fmt.Errorf("resolve backup input path: %w", err)
	}
	if databasePath, err = filepath.Abs(databasePath); err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	if credentialKeyPath == "" {
		credentialKeyPath = databasePath + ".auth.key"
	} else if credentialKeyPath, err = filepath.Abs(credentialKeyPath); err != nil {
		return fmt.Errorf("resolve credential key path: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := backup.Restore(ctx, backup.RestoreOptions{
		InputPath: inputPath, DatabasePath: databasePath,
		CredentialKeyPath: credentialKeyPath, Force: force,
	})
	if err != nil {
		return err
	}
	logger.Info("backup restored", "path", inputPath, "database", databasePath,
		"created_at", result.Manifest.CreatedAt, "recovery_copies", result.Replaced)
	return nil
}

func runServe(args []string, logger *slog.Logger) error {
	cfg, err := config.Load(args, os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app, err := buildApplication(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer app.Close()

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           app.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverError := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", cfg.Address)
		serverError <- server.ListenAndServe()
	}()

	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		forced, err := shutdownHTTPServer(server, 10*time.Second)
		if err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		if forced {
			logger.Warn("graceful HTTP shutdown timed out; active connections were closed")
		}
		return nil
	}
}

func runBackup(args []string, logger *slog.Logger) error {
	databasePath := "peerphonic.db"
	if configured, ok := os.LookupEnv("PEERPHONIC_DATABASE"); ok {
		databasePath = configured
	}
	var outputPath, outputDirectory, credentialKeyPath string
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&databasePath, "database", databasePath, "SQLite database path")
	flags.StringVar(&credentialKeyPath, "credential-key", "", "credential encryption key path")
	flags.StringVar(&outputPath, "output", "", "backup archive path")
	flags.StringVar(&outputDirectory, "output-dir", "", "directory for a timestamped backup archive")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if (outputPath == "") == (outputDirectory == "") {
		return errors.New("set exactly one of --output or --output-dir")
	}
	var err error
	databasePath, err = filepath.Abs(databasePath)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	if outputPath != "" {
		outputPath, err = filepath.Abs(outputPath)
		if err != nil {
			return fmt.Errorf("resolve backup output path: %w", err)
		}
	} else {
		outputDirectory, err = filepath.Abs(outputDirectory)
		if err != nil {
			return fmt.Errorf("resolve backup output directory: %w", err)
		}
		name := "peerphonic-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".tar.gz"
		outputPath = filepath.Join(outputDirectory, name)
	}
	if credentialKeyPath == "" {
		credentialKeyPath = databasePath + ".auth.key"
	} else if credentialKeyPath, err = filepath.Abs(credentialKeyPath); err != nil {
		return fmt.Errorf("resolve credential key path: %w", err)
	}
	snapshotter, err := sqlite.NewSnapshotter(databasePath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manifest, err := backup.Create(ctx, snapshotter, backup.Options{
		OutputPath: outputPath, CredentialKeyPath: credentialKeyPath,
	})
	if err != nil {
		return err
	}
	logger.Info("backup created", "path", outputPath, "files", len(manifest.Files))
	return nil
}

func shutdownHTTPServer(server *http.Server, timeout time.Duration) (bool, error) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err == nil {
		return false, nil
	}
	if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return true, err
	}
	return true, nil
}
