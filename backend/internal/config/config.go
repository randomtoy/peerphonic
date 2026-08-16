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
	Address  string `json:"address"`
	MusicDir string `json:"music_dir"`
	Database string `json:"database"`
	CacheDir string `json:"cache_dir"`
	Username string `json:"username"`
	Password string `json:"password"`
	Scan     bool   `json:"scan_on_start"`
}

func Defaults() Config {
	return Config{
		Address:  ":8080",
		Database: "peerphonic.db",
		CacheDir: "cache",
		Username: "admin",
		Password: "admin",
		Scan:     true,
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
	flags.StringVar(&cfg.Username, "username", cfg.Username, "OpenSubsonic username")
	flags.StringVar(&cfg.Password, "password", cfg.Password, "OpenSubsonic password")
	flags.BoolVar(&cfg.Scan, "scan", cfg.Scan, "scan music directory on startup")
	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if cfg.MusicDir == "" {
		return Config{}, errors.New("music directory is required (use --music or PEERPHONIC_MUSIC_DIR)")
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
		"PEERPHONIC_ADDRESS":   &cfg.Address,
		"PEERPHONIC_MUSIC_DIR": &cfg.MusicDir,
		"PEERPHONIC_DATABASE":  &cfg.Database,
		"PEERPHONIC_CACHE_DIR": &cfg.CacheDir,
		"PEERPHONIC_USERNAME":  &cfg.Username,
		"PEERPHONIC_PASSWORD":  &cfg.Password,
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
	return nil
}
