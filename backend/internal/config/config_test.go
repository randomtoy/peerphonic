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
		"PEERPHONIC_ADDRESS":                                 ":2000",
		"PEERPHONIC_USERNAME":                                "env",
		"PEERPHONIC_CACHE_SIZE_BYTES":                        "2048",
		"PEERPHONIC_TORRENT_SEED":                            "false",
		"PEERPHONIC_TORRENT_PORT":                            "43000",
		"PEERPHONIC_TORRENT_UPLOAD_LIMIT_BYTES_PER_SECOND":   "1024",
		"PEERPHONIC_TORRENT_DOWNLOAD_LIMIT_BYTES_PER_SECOND": "2048",
		"PEERPHONIC_TORRENT_MAX_ACTIVE_DOWNLOADS":            "5",
		"PEERPHONIC_SCAN_INTERVAL_SECONDS":                   "60",
		"PEERPHONIC_SLSKD_URL":                               "http://slskd-env:5030",
		"PEERPHONIC_SLSKD_API_KEY":                           "0123456789abcdef",
		"PEERPHONIC_SLSKD_TIMEOUT_SECONDS":                   "7",
		"PEERPHONIC_SLSKD_DOWNLOADS_DIR":                     filepath.Join(dir, "env-downloads"),
		"PEERPHONIC_SLSKD_INCOMPLETE_DIR":                    filepath.Join(dir, "env-incomplete"),
		"PEERPHONIC_SLSKD_MAX_ACTIVE_DOWNLOADS":              "4",
		"PEERPHONIC_SLSKD_RETRY_ATTEMPTS":                    "6",
		"PEERPHONIC_FFMPEG_PATH":                             "ffmpeg-env",
		"PEERPHONIC_AUTH_FAILURE_LIMIT":                      "12",
		"PEERPHONIC_AUTH_FAILURE_WINDOW_SECONDS":             "90",
		"PEERPHONIC_AUTH_BLOCK_SECONDS":                      "120",
	}
	lookup := func(key string) (string, bool) { value, ok := env[key]; return value, ok }

	cfg, err := Load([]string{
		"--config", configPath,
		"--address", ":3000",
		"--cache-size", "4096",
		"--torrent-seed=true",
		"--torrent-port", "44000",
		"--torrent-port-forwarding=true",
		"--torrent-upload-limit", "4096",
		"--torrent-download-limit", "8192",
		"--torrent-max-downloads", "7",
		"--scan-interval", "120",
		"--slskd-url", "http://slskd-flag:5030",
		"--slskd-timeout", "9",
		"--slskd-downloads", filepath.Join(dir, "flag-downloads"),
		"--slskd-max-downloads", "8",
		"--slskd-retry-attempts", "9",
		"--ffmpeg", "ffmpeg-flag",
		"--auth-failure-limit", "15",
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
	if cfg.TorrentUploadLimit != 4096 || cfg.TorrentDownloadLimit != 8192 {
		t.Errorf("torrent limits = upload %d, download %d", cfg.TorrentUploadLimit, cfg.TorrentDownloadLimit)
	}
	if cfg.TorrentMaxDownloads != 7 {
		t.Errorf("TorrentMaxDownloads = %d, want 7", cfg.TorrentMaxDownloads)
	}
	if cfg.ScanIntervalSeconds != 120 {
		t.Errorf("ScanIntervalSeconds = %d, want 120", cfg.ScanIntervalSeconds)
	}
	if cfg.SlskdURL != "http://slskd-flag:5030" || cfg.SlskdAPIKey != "0123456789abcdef" || cfg.SlskdTimeoutSeconds != 9 {
		t.Errorf("slskd settings = URL %q, key %q, timeout %d", cfg.SlskdURL, cfg.SlskdAPIKey, cfg.SlskdTimeoutSeconds)
	}
	if cfg.SlskdDownloadsDir != filepath.Join(dir, "flag-downloads") ||
		cfg.SlskdIncompleteDir != filepath.Join(dir, "env-incomplete") {
		t.Errorf("slskd directories = downloads %q, incomplete %q", cfg.SlskdDownloadsDir, cfg.SlskdIncompleteDir)
	}
	if cfg.SlskdMaxDownloads != 8 || cfg.SlskdRetryAttempts != 9 {
		t.Errorf("slskd download policy = max %d, retries %d", cfg.SlskdMaxDownloads, cfg.SlskdRetryAttempts)
	}
	if cfg.FFmpegPath != "ffmpeg-flag" {
		t.Errorf("FFmpegPath = %q, want ffmpeg-flag", cfg.FFmpegPath)
	}
	if cfg.AuthFailureLimit != 15 || cfg.AuthFailureWindow != 90 || cfg.AuthBlockSeconds != 120 {
		t.Errorf("auth limit = %d failures/%ds, block %ds", cfg.AuthFailureLimit, cfg.AuthFailureWindow, cfg.AuthBlockSeconds)
	}
}

func TestLoadRejectsInvalidAuthenticationRateLimit(t *testing.T) {
	t.Parallel()

	if _, err := Load([]string{"--music", t.TempDir(), "--auth-failure-limit", "0"}, func(string) (string, bool) {
		return "", false
	}); err == nil {
		t.Fatal("Load() error = nil, want invalid authentication rate limit error")
	}
}

func TestLoadPostgresMetadataConfiguration(t *testing.T) {
	t.Parallel()

	credentialKey := filepath.Join(t.TempDir(), "credentials.key")
	cfg, err := Load([]string{
		"--music", t.TempDir(), "--metadata-driver", "postgres",
		"--database-url", "postgres://peerphonic:secret@postgres:5432/peerphonic?sslmode=disable",
		"--credential-key", credentialKey,
	}, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MetadataDriver != "postgres" || cfg.DatabaseURL == "" || cfg.CredentialKeyPath != credentialKey {
		t.Fatalf("PostgreSQL config = %#v", cfg)
	}
}

func TestLoadPostgresRequiresConnectionURL(t *testing.T) {
	t.Parallel()

	if _, err := Load([]string{
		"--music", t.TempDir(), "--metadata-driver", "postgres",
	}, func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("Load() error = nil, want missing PostgreSQL URL error")
	}
}

func TestLoadDerivesSlskdDirectoriesFromCache(t *testing.T) {
	t.Parallel()

	cache := filepath.Join(t.TempDir(), "cache")
	cfg, err := Load([]string{"--music", t.TempDir(), "--cache", cache}, func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SlskdDownloadsDir != filepath.Join(cache, "soulseek", "downloads") ||
		cfg.SlskdIncompleteDir != filepath.Join(cache, "soulseek", "incomplete") {
		t.Fatalf("slskd directories = downloads %q, incomplete %q", cfg.SlskdDownloadsDir, cfg.SlskdIncompleteDir)
	}
	if cfg.SlskdTimeoutSeconds != 15 {
		t.Fatalf("default slskd timeout = %d, want 15", cfg.SlskdTimeoutSeconds)
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
	if !cfg.TorrentSeed || cfg.TorrentPort != 42069 || cfg.TorrentPortForwarding || cfg.ScanIntervalSeconds != 300 {
		t.Fatalf("torrent defaults = seed %t, port %d, forwarding %t", cfg.TorrentSeed, cfg.TorrentPort, cfg.TorrentPortForwarding)
	}
}

func TestLoadRejectsNegativeScanInterval(t *testing.T) {
	t.Parallel()

	_, err := Load([]string{"--music", t.TempDir(), "--scan-interval", "-1"}, func(string) (string, bool) {
		return "", false
	})
	if err == nil {
		t.Fatal("Load() error = nil, want invalid scan interval error")
	}
}

func TestLoadRejectsNegativeTorrentTransferLimit(t *testing.T) {
	t.Parallel()

	_, err := Load([]string{"--music", t.TempDir(), "--torrent-upload-limit", "-1"}, func(string) (string, bool) {
		return "", false
	})
	if err == nil {
		t.Fatal("Load() error = nil, want invalid torrent transfer limit error")
	}
}

func TestLoadRejectsInvalidSlskdTimeout(t *testing.T) {
	t.Parallel()

	_, err := Load([]string{"--music", t.TempDir(), "--slskd-timeout", "0"}, func(string) (string, bool) {
		return "", false
	})
	if err == nil {
		t.Fatal("Load() error = nil, want invalid slskd timeout error")
	}
}

func TestLoadRejectsInvalidDownloadConcurrency(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"--music", t.TempDir(), "--torrent-max-downloads", "0"},
		{"--music", t.TempDir(), "--slskd-max-downloads", "0"},
		{"--music", t.TempDir(), "--slskd-retry-attempts", "0"},
	} {
		if _, err := Load(arguments, func(string) (string, bool) { return "", false }); err == nil {
			t.Fatalf("Load(%v) error = nil", arguments)
		}
	}
}
