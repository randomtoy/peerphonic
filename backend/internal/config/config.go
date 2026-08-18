package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Address               string `json:"address"`
	MusicDir              string `json:"music_dir"`
	Database              string `json:"database"`
	CacheDir              string `json:"cache_dir"`
	CacheSizeBytes        int64  `json:"cache_size_bytes"`
	TorrentDir            string `json:"torrent_dir"`
	TorrentSeed           bool   `json:"torrent_seed"`
	TorrentPort           int    `json:"torrent_port"`
	TorrentPortForwarding bool   `json:"torrent_port_forwarding"`
	TorrentUploadLimit    int64  `json:"torrent_upload_limit_bytes_per_second"`
	TorrentDownloadLimit  int64  `json:"torrent_download_limit_bytes_per_second"`
	Username              string `json:"username"`
	Password              string `json:"password"`
	Scan                  bool   `json:"scan_on_start"`
	ScanIntervalSeconds   int    `json:"scan_interval_seconds"`
	SlskdURL              string `json:"slskd_url"`
	SlskdAPIKey           string `json:"slskd_api_key"`
	SlskdTimeoutSeconds   int    `json:"slskd_timeout_seconds"`
	SlskdDownloadsDir     string `json:"slskd_downloads_dir"`
	SlskdIncompleteDir    string `json:"slskd_incomplete_dir"`
}

func Defaults() Config {
	return Config{
		Address:             ":8080",
		Database:            "peerphonic.db",
		CacheDir:            "cache",
		CacheSizeBytes:      10 << 30,
		TorrentDir:          "torrents",
		TorrentSeed:         true,
		TorrentPort:         42069,
		Username:            "admin",
		Password:            "admin",
		Scan:                true,
		ScanIntervalSeconds: 300,
		SlskdTimeoutSeconds: 15,
	}
}

// Load applies configuration in increasing precedence: defaults, JSON file,
// PEERPHONIC_* environment variables, then command-line flags.
func Load(args []string, lookupEnv func(string) (string, bool)) (Config, error) {
	cfg := Defaults()
	configPath := findConfigPath(args)
	if configPath == "" {
		if value, ok := lookupEnv("PEERPHONIC_CONFIG"); ok {
			configPath = value
		}
	}
	if configPath != "" {
		if err := readFile(configPath, &cfg); err != nil {
			return Config{}, err
		}
	}
	if err := applyEnv(&cfg, lookupEnv); err != nil {
		return Config{}, err
	}

	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&configPath, "config", configPath, "path to JSON configuration")
	flags.StringVar(&cfg.Address, "address", cfg.Address, "HTTP listen address")
	flags.StringVar(&cfg.MusicDir, "music", cfg.MusicDir, "music library directory")
	flags.StringVar(&cfg.Database, "database", cfg.Database, "SQLite database path")
	flags.StringVar(&cfg.CacheDir, "cache", cfg.CacheDir, "media cache directory")
	flags.Int64Var(&cfg.CacheSizeBytes, "cache-size", cfg.CacheSizeBytes, "maximum media cache size in bytes")
	flags.StringVar(&cfg.TorrentDir, "torrents", cfg.TorrentDir, "directory containing torrent metadata")
	flags.BoolVar(&cfg.TorrentSeed, "torrent-seed", cfg.TorrentSeed, "upload verified torrent pieces")
	flags.IntVar(&cfg.TorrentPort, "torrent-port", cfg.TorrentPort, "BitTorrent listen port (0 chooses a random port)")
	flags.BoolVar(&cfg.TorrentPortForwarding, "torrent-port-forwarding", cfg.TorrentPortForwarding, "enable UPnP/NAT-PMP torrent port forwarding")
	flags.Int64Var(&cfg.TorrentUploadLimit, "torrent-upload-limit", cfg.TorrentUploadLimit, "torrent upload limit in bytes per second (0 is unlimited)")
	flags.Int64Var(&cfg.TorrentDownloadLimit, "torrent-download-limit", cfg.TorrentDownloadLimit, "torrent download limit in bytes per second (0 is unlimited)")
	flags.StringVar(&cfg.Username, "username", cfg.Username, "OpenSubsonic username")
	flags.StringVar(&cfg.Password, "password", cfg.Password, "OpenSubsonic password")
	flags.BoolVar(&cfg.Scan, "scan", cfg.Scan, "scan music directory on startup")
	flags.IntVar(&cfg.ScanIntervalSeconds, "scan-interval", cfg.ScanIntervalSeconds, "background library scan interval in seconds (0 disables it)")
	flags.StringVar(&cfg.SlskdURL, "slskd-url", cfg.SlskdURL, "slskd HTTP API base URL")
	flags.IntVar(&cfg.SlskdTimeoutSeconds, "slskd-timeout", cfg.SlskdTimeoutSeconds, "slskd API timeout in seconds")
	flags.StringVar(&cfg.SlskdDownloadsDir, "slskd-downloads", cfg.SlskdDownloadsDir, "shared slskd completed downloads directory")
	flags.StringVar(&cfg.SlskdIncompleteDir, "slskd-incomplete", cfg.SlskdIncompleteDir, "shared slskd incomplete downloads directory")
	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if cfg.MusicDir == "" {
		return Config{}, errors.New("music directory is required (use --music or PEERPHONIC_MUSIC_DIR)")
	}
	if cfg.CacheSizeBytes <= 0 {
		return Config{}, errors.New("media cache size must be positive")
	}
	if cfg.TorrentPort < 0 || cfg.TorrentPort > 65535 {
		return Config{}, errors.New("torrent port must be between 0 and 65535")
	}
	if cfg.TorrentUploadLimit < 0 || cfg.TorrentDownloadLimit < 0 {
		return Config{}, errors.New("torrent transfer limits must be non-negative")
	}
	if cfg.ScanIntervalSeconds < 0 {
		return Config{}, errors.New("scan interval must be non-negative")
	}
	if cfg.SlskdTimeoutSeconds <= 0 {
		return Config{}, errors.New("slskd timeout must be positive")
	}

	var err error
	cfg.MusicDir, err = filepath.Abs(cfg.MusicDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve music directory: %w", err)
	}
	cfg.Database, err = filepath.Abs(cfg.Database)
	if err != nil {
		return Config{}, fmt.Errorf("resolve database path: %w", err)
	}
	cfg.CacheDir, err = filepath.Abs(cfg.CacheDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve cache directory: %w", err)
	}
	if cfg.SlskdDownloadsDir == "" {
		cfg.SlskdDownloadsDir = filepath.Join(cfg.CacheDir, "soulseek", "downloads")
	} else if cfg.SlskdDownloadsDir, err = filepath.Abs(cfg.SlskdDownloadsDir); err != nil {
		return Config{}, fmt.Errorf("resolve slskd downloads directory: %w", err)
	}
	if cfg.SlskdIncompleteDir == "" {
		cfg.SlskdIncompleteDir = filepath.Join(cfg.CacheDir, "soulseek", "incomplete")
	} else if cfg.SlskdIncompleteDir, err = filepath.Abs(cfg.SlskdIncompleteDir); err != nil {
		return Config{}, fmt.Errorf("resolve slskd incomplete directory: %w", err)
	}
	cfg.TorrentDir, err = filepath.Abs(cfg.TorrentDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve torrent metadata directory: %w", err)
	}
	return cfg, nil
}

func findConfigPath(args []string) string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config=")
		}
		if arg == "--config" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func readFile(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("decode config file: %w", err)
	}
	return nil
}

func applyEnv(cfg *Config, lookup func(string) (string, bool)) error {
	for key, target := range map[string]*string{
		"PEERPHONIC_ADDRESS":              &cfg.Address,
		"PEERPHONIC_MUSIC_DIR":            &cfg.MusicDir,
		"PEERPHONIC_DATABASE":             &cfg.Database,
		"PEERPHONIC_CACHE_DIR":            &cfg.CacheDir,
		"PEERPHONIC_TORRENT_DIR":          &cfg.TorrentDir,
		"PEERPHONIC_USERNAME":             &cfg.Username,
		"PEERPHONIC_PASSWORD":             &cfg.Password,
		"PEERPHONIC_SLSKD_URL":            &cfg.SlskdURL,
		"PEERPHONIC_SLSKD_API_KEY":        &cfg.SlskdAPIKey,
		"PEERPHONIC_SLSKD_DOWNLOADS_DIR":  &cfg.SlskdDownloadsDir,
		"PEERPHONIC_SLSKD_INCOMPLETE_DIR": &cfg.SlskdIncompleteDir,
	} {
		if value, ok := lookup(key); ok {
			*target = value
		}
	}
	if value, ok := lookup("PEERPHONIC_SCAN_ON_START"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_SCAN_ON_START: %w", err)
		}
		cfg.Scan = parsed
	}
	if value, ok := lookup("PEERPHONIC_SCAN_INTERVAL_SECONDS"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_SCAN_INTERVAL_SECONDS: %w", err)
		}
		cfg.ScanIntervalSeconds = parsed
	}
	if value, ok := lookup("PEERPHONIC_SLSKD_TIMEOUT_SECONDS"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_SLSKD_TIMEOUT_SECONDS: %w", err)
		}
		cfg.SlskdTimeoutSeconds = parsed
	}
	if value, ok := lookup("PEERPHONIC_TORRENT_SEED"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_TORRENT_SEED: %w", err)
		}
		cfg.TorrentSeed = parsed
	}
	if value, ok := lookup("PEERPHONIC_TORRENT_PORT_FORWARDING"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_TORRENT_PORT_FORWARDING: %w", err)
		}
		cfg.TorrentPortForwarding = parsed
	}
	if value, ok := lookup("PEERPHONIC_CACHE_SIZE_BYTES"); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_CACHE_SIZE_BYTES: %w", err)
		}
		cfg.CacheSizeBytes = parsed
	}
	if value, ok := lookup("PEERPHONIC_TORRENT_PORT"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_TORRENT_PORT: %w", err)
		}
		cfg.TorrentPort = parsed
	}
	if value, ok := lookup("PEERPHONIC_TORRENT_UPLOAD_LIMIT_BYTES_PER_SECOND"); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_TORRENT_UPLOAD_LIMIT_BYTES_PER_SECOND: %w", err)
		}
		cfg.TorrentUploadLimit = parsed
	}
	if value, ok := lookup("PEERPHONIC_TORRENT_DOWNLOAD_LIMIT_BYTES_PER_SECOND"); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("parse PEERPHONIC_TORRENT_DOWNLOAD_LIMIT_BYTES_PER_SECOND: %w", err)
		}
		cfg.TorrentDownloadLimit = parsed
	}
	return nil
}
