# Configuration reference

Peerphonic applies configuration in this order, with later sources taking
precedence:

1. built-in defaults;
2. JSON configuration file;
3. `PEERPHONIC_*` environment variables;
4. command-line flags.

Pass a JSON file with `--config peerphonic.json` or set `PEERPHONIC_CONFIG`.
See [`backend/config.example.json`](../backend/config.example.json) for a complete
example.

## Server and storage

| JSON field | Environment | CLI | Default |
| --- | --- | --- | --- |
| `address` | `PEERPHONIC_ADDRESS` | `--address` | `:8080` |
| `music_dir` | `PEERPHONIC_MUSIC_DIR` | `--music` | required |
| `metadata_driver` | `PEERPHONIC_METADATA_DRIVER` | `--metadata-driver` | `sqlite` |
| `database` | `PEERPHONIC_DATABASE` | `--database` | `peerphonic.db` |
| `database_url` | `PEERPHONIC_DATABASE_URL` | `--database-url` | empty |
| `credential_key_path` | `PEERPHONIC_CREDENTIAL_KEY_PATH` | `--credential-key` | `<database>.auth.key` |
| `cache_dir` | `PEERPHONIC_CACHE_DIR` | `--cache` | `cache` |
| `cache_size_bytes` | `PEERPHONIC_CACHE_SIZE_BYTES` | `--cache-size` | 10 GiB |
| `scan_on_start` | `PEERPHONIC_SCAN_ON_START` | `--scan` | `true` |
| `scan_interval_seconds` | `PEERPHONIC_SCAN_INTERVAL_SECONDS` | `--scan-interval` | `300` |
| `ffmpeg_path` | `PEERPHONIC_FFMPEG_PATH` | `--ffmpeg` | `ffmpeg` |

PostgreSQL requires `metadata_driver=postgres` and a `database_url`. SQLite is
the zero-configuration default. An empty FFmpeg path disables transcoding.

## Bootstrap authentication

| JSON field | Environment | CLI | Default |
| --- | --- | --- | --- |
| `username` | `PEERPHONIC_USERNAME` | `--username` | `admin` |
| `password` | `PEERPHONIC_PASSWORD` | `--password` | `admin` |
| `auth_failure_limit` | `PEERPHONIC_AUTH_FAILURE_LIMIT` | `--auth-failure-limit` | `10` |
| `auth_failure_window_seconds` | `PEERPHONIC_AUTH_FAILURE_WINDOW_SECONDS` | `--auth-failure-window` | `60` |
| `auth_block_seconds` | `PEERPHONIC_AUTH_BLOCK_SECONDS` | `--auth-block-time` | `60` |

Never expose a first start with the default password. Bootstrap values create
the initial account only when no users exist; they do not reset database users.
The credential key protects reversible credentials needed for OpenSubsonic token
authentication and must be backed up with the metadata database.

## BitTorrent

| JSON field | Environment | CLI | Default |
| --- | --- | --- | --- |
| `torrent_dir` | `PEERPHONIC_TORRENT_DIR` | `--torrents` | `torrents` |
| `torrent_seed` | `PEERPHONIC_TORRENT_SEED` | `--torrent-seed` | `true` |
| `torrent_port` | `PEERPHONIC_TORRENT_PORT` | `--torrent-port` | `42069` |
| `torrent_port_forwarding` | `PEERPHONIC_TORRENT_PORT_FORWARDING` | `--torrent-port-forwarding` | `false` |
| `torrent_upload_limit_bytes_per_second` | `PEERPHONIC_TORRENT_UPLOAD_LIMIT_BYTES_PER_SECOND` | `--torrent-upload-limit` | `0` |
| `torrent_download_limit_bytes_per_second` | `PEERPHONIC_TORRENT_DOWNLOAD_LIMIT_BYTES_PER_SECOND` | `--torrent-download-limit` | `0` |
| `torrent_max_active_downloads` | `PEERPHONIC_TORRENT_MAX_ACTIVE_DOWNLOADS` | `--torrent-max-downloads` | `3` |

Port `0` selects a random listen port. Transfer limit `0` means unlimited.
Transfer limits saved through the dashboard or API override startup values on
later runs.

## Soulseek and slskd

| JSON field | Environment | CLI | Default |
| --- | --- | --- | --- |
| `slskd_url` | `PEERPHONIC_SLSKD_URL` | `--slskd-url` | disabled |
| `slskd_api_key` | `PEERPHONIC_SLSKD_API_KEY` | — | empty |
| `slskd_timeout_seconds` | `PEERPHONIC_SLSKD_TIMEOUT_SECONDS` | `--slskd-timeout` | `15` |
| `slskd_downloads_dir` | `PEERPHONIC_SLSKD_DOWNLOADS_DIR` | `--slskd-downloads` | `<cache>/soulseek/downloads` |
| `slskd_incomplete_dir` | `PEERPHONIC_SLSKD_INCOMPLETE_DIR` | `--slskd-incomplete` | `<cache>/soulseek/incomplete` |
| `slskd_max_active_downloads` | `PEERPHONIC_SLSKD_MAX_ACTIVE_DOWNLOADS` | `--slskd-max-downloads` | `2` |
| `slskd_retry_attempts` | `PEERPHONIC_SLSKD_RETRY_ATTEMPTS` | `--slskd-retry-attempts` | `3` |
| `slskd_prebuffer_bytes` | `PEERPHONIC_SLSKD_PREBUFFER_BYTES` | `--slskd-prebuffer-bytes` | `1048576` |
| `slskd_prebuffer_timeout_seconds` | `PEERPHONIC_SLSKD_PREBUFFER_TIMEOUT_SECONDS` | `--slskd-prebuffer-timeout` | `15` |

Peerphonic authenticates to slskd with `X-API-Key`. Use HTTPS or a private
container network because the API key is long-lived. Downloads and incomplete
paths must refer to storage shared with slskd.
