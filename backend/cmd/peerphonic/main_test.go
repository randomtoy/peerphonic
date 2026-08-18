package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
)

func TestRunBackupCreatesArchiveWhileDatabaseIsOpen(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	databasePath := filepath.Join(root, "peerphonic.db")
	catalog, err := sqlite.Open(context.Background(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	if err := os.WriteFile(databasePath+".auth.key", make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(root, "backups", "peerphonic.tar.gz")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := run([]string{
		"backup", "--database", databasePath, "--output", outputPath,
	}, logger); err != nil {
		t.Fatalf("run(backup) error = %v", err)
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup info = %#v, error = %v", info, err)
	}
}

func TestShutdownHTTPServerGracefullyStopsIdleServer(t *testing.T) {
	t.Parallel()

	server, address, served := startHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	response, err := http.Get(address)
	if err != nil {
		t.Fatalf("GET server: %v", err)
	}
	response.Body.Close()

	forced, err := shutdownHTTPServer(server, time.Second)
	if err != nil || forced {
		t.Fatalf("shutdown forced = %t, error = %v", forced, err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestShutdownHTTPServerClosesLongRunningRequestsAfterTimeout(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	finished := make(chan struct{})
	server, address, served := startHTTPServer(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
		close(finished)
	}))
	requestFinished := make(chan struct{})
	go func() {
		defer close(requestFinished)
		response, err := http.Get(address)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach server")
	}

	forced, err := shutdownHTTPServer(server, 10*time.Millisecond)
	if err != nil || !forced {
		t.Fatalf("shutdown forced = %t, error = %v", forced, err)
	}
	for name, channel := range map[string]<-chan struct{}{
		"handler": finished, "request": requestFinished,
	} {
		select {
		case <-channel:
		case <-time.After(time.Second):
			t.Fatalf("%s did not stop", name)
		}
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve() error = %v", err)
	}
}

func startHTTPServer(t *testing.T, handler http.Handler) (*http.Server, string, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	t.Cleanup(func() { _ = server.Close() })
	served := make(chan error, 1)
	go func() {
		served <- server.Serve(listener)
	}()
	return server, "http://" + listener.Addr().String(), served
}
