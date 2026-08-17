# Peerphonic

Peerphonic is a self-hosted music server with OpenSubsonic compatibility. The
current milestone serves a local music library. Its core is designed so cache,
self-hosted storage, friends, Soulseek, and other P2P sources can be added without
changing the client-facing streaming flow.

## Current capabilities

- recursive scanning of MP3, FLAC, Ogg/Opus, M4A, AAC, and WAV files;
- tag extraction with directory/filename fallbacks;
- a migrated SQLite metadata catalog;
- XML and JSON OpenSubsonic responses;
- password, hex-encoded password, and token/salt authentication;
- artist/album/track browsing and HTTP range streaming;
- a small Peerphonic health endpoint at `/api/v1/health`.

The implemented OpenSubsonic endpoints are:

```text
/rest/ping[.view]
/rest/getLicense[.view]
/rest/getMusicFolders[.view]
/rest/getIndexes[.view]
/rest/getMusicDirectory[.view]
/rest/getGenres[.view]
/rest/getArtists[.view]
/rest/getArtist[.view]
/rest/getAlbumList2[.view]
/rest/getAlbum[.view]
/rest/getSong[.view]
/rest/search3[.view]
/rest/getPlaylists[.view]
/rest/getOpenSubsonicExtensions[.view]
/rest/getScanStatus[.view]
/rest/startScan[.view]
/rest/stream[.view]
/rest/download[.view]
```

## Quick start

Go 1.25 or newer is required.

```bash
cd backend
go build -o peerphonic ./cmd/peerphonic
./peerphonic serve --music /path/to/Music
```

The server listens on `:8080`, stores its catalog in `peerphonic.db`, and uses
`admin` / `admin` by default. Set a private password before exposing it to a
network:

```bash
PEERPHONIC_USERNAME=music \
PEERPHONIC_PASSWORD='replace-this-password' \
./peerphonic serve --music /path/to/Music
```

An OpenSubsonic client can then connect to `http://localhost:8080` with those
credentials. A direct connectivity check is also available:

```bash
curl 'http://localhost:8080/rest/ping?u=music&p=replace-this-password&v=1.16.1&c=curl&f=json'
```

## Configuration

Configuration is applied in this order: defaults, JSON config file, environment,
then CLI flags.

| JSON field | Environment | CLI | Default |
| --- | --- | --- | --- |
| `address` | `PEERPHONIC_ADDRESS` | `--address` | `:8080` |
| `music_dir` | `PEERPHONIC_MUSIC_DIR` | `--music` | required |
| `database` | `PEERPHONIC_DATABASE` | `--database` | `peerphonic.db` |
| `cache_dir` | `PEERPHONIC_CACHE_DIR` | `--cache` | `cache` |
| `username` | `PEERPHONIC_USERNAME` | `--username` | `admin` |
| `password` | `PEERPHONIC_PASSWORD` | `--password` | `admin` |
| `scan_on_start` | `PEERPHONIC_SCAN_ON_START` | `--scan` | `true` |

Use a config file with `--config peerphonic.json` or set its path through
`PEERPHONIC_CONFIG`. See [`backend/config.example.json`](backend/config.example.json).

## Architecture

Peerphonic uses a hexagonal architecture. Domain and application services depend
on small ports, while SQLite, local files, tag readers, HTTP, cache storage, and
eventual P2P transports remain adapters.

```text
OpenSubsonic / Peerphonic API
              |
       application services
              |
       domain + core ports
          /         \
SQLite catalog    source providers
                  local now, P2P later
```

Metadata and media/blob storage are separate boundaries. The OpenSubsonic stream
handler resolves a domain source through `StreamingService`; it does not know
whether bytes are local, cached, or remote.

## Development

```bash
cd backend
go test ./...
go vet ./...
```

The `web/` directory is reserved for an optional Peerphonic-specific frontend.
The backend does not depend on it. Container deployment files live in `deploy/`.
