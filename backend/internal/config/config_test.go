package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"music_dir":"from-file","address":":1000","username":"file"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"PEERPHONIC_ADDRESS":          ":2000",
		"PEERPHONIC_USERNAME":         "env",
		"PEERPHONIC_CACHE_SIZE_BYTES": "2048",
		"PEERPHONIC_TORRENT_SEED":     "false",
		"PEERPHONIC_TORRENT_PORT":     "43000",
	}
	lookup := func(key string) (string, bool) { value, ok := env[key]; return value, ok }

	cfg, err := Load([]string{
		"--config", configPath,
		"--address", ":3000",
		"--cache-size", "4096",
		"--torrent-seed=true",
		"--torrent-port", "44000",
		"--torrent-port-forwarding=true",
	}, lookup)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Address != ":3000" {
		t.Errorf("Address = %q, want :3000", cfg.Address)
	}
	if cfg.Username != "env" {
		t.Errorf("Username = %q, want env", cfg.Username)
	}
	if filepath.Base(cfg.MusicDir) != "from-file" {
		t.Errorf("MusicDir = %q, want path ending in from-file", cfg.MusicDir)
	}
	if cfg.CacheSizeBytes != 4096 {
		t.Errorf("CacheSizeBytes = %d, want 4096", cfg.CacheSizeBytes)
	}
	if filepath.Base(cfg.TorrentDir) != "torrents" {
		t.Errorf("TorrentDir = %q, want path ending in torrents", cfg.TorrentDir)
	}
	if !cfg.TorrentSeed || cfg.TorrentPort != 44000 || !cfg.TorrentPortForwarding {
		t.Errorf("torrent settings = seed %t, port %d, forwarding %t", cfg.TorrentSeed, cfg.TorrentPort, cfg.TorrentPortForwarding)
	}
}

func TestLoadRejectsInvalidCacheSize(t *testing.T) {
	t.Parallel()

	_, err := Load([]string{"--music", t.TempDir()}, func(key string) (string, bool) {
		if key == "PEERPHONIC_CACHE_SIZE_BYTES" {
			return "many", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("Load() error = nil, want invalid cache size error")
	}
}

func TestLoadRequiresMusicDirectory(t *testing.T) {
	t.Parallel()

	_, err := Load(nil, func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("Load() error = nil, want missing music directory error")
	}
}

func TestLoadRejectsInvalidTorrentPort(t *testing.T) {
	t.Parallel()

	_, err := Load([]string{"--music", t.TempDir(), "--torrent-port", "65536"}, func(string) (string, bool) {
		return "", false
	})
	if err == nil {
		t.Fatal("Load() error = nil, want invalid torrent port error")
	}
}

func TestDefaultsEnableTorrentSeeding(t *testing.T) {
	t.Parallel()

	cfg := Defaults()
	if !cfg.TorrentSeed || cfg.TorrentPort != 42069 || cfg.TorrentPortForwarding {
		t.Fatalf("torrent defaults = seed %t, port %d, forwarding %t", cfg.TorrentSeed, cfg.TorrentPort, cfg.TorrentPortForwarding)
	}
}
