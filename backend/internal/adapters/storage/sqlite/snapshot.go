package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
)

type Snapshotter struct {
	source string
}

func NewSnapshotter(source string) (*Snapshotter, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("stat sqlite database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sqlite database %q is not a regular file", source)
	}
	return &Snapshotter{source: source}, nil
}

func (s *Snapshotter) Snapshot(ctx context.Context, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("snapshot destination %q already exists", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect snapshot destination: %w", err)
	}
	database, err := sql.Open("sqlite", readOnlySQLiteDSN(s.source))
	if err != nil {
		return fmt.Errorf("open sqlite database for snapshot: %w", err)
	}
	database.SetMaxOpenConns(1)
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database for snapshot: %w", err)
	}
	if _, err := database.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		return fmt.Errorf("create sqlite snapshot: %w", err)
	}
	if err := verifySQLiteSnapshot(ctx, destination); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

func verifySQLiteSnapshot(ctx context.Context, path string) error {
	database, err := sql.Open("sqlite", readOnlySQLiteDSN(path))
	if err != nil {
		return fmt.Errorf("open sqlite snapshot: %w", err)
	}
	defer database.Close()
	var result string
	if err := database.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("check sqlite snapshot: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("sqlite snapshot integrity check returned %q", result)
	}
	return nil
}

func readOnlySQLiteDSN(path string) string {
	location := &url.URL{Scheme: "file", Path: path}
	query := location.Query()
	query.Set("mode", "ro")
	location.RawQuery = query.Encode()
	return location.String()
}
