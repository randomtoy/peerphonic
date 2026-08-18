package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: peerphonic serve --music /path/to/music")
	}
	cfg, err := config.Load(args[1:], os.LookupEnv)
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
