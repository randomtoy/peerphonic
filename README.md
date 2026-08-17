# Peerphonic

Peerphonic is a self-hosted music server with OpenSubsonic compatibility. The
current milestone serves a local music library. Its core is designed so cache,
self-hosted storage, friends, Soulseek, and other P2P sources can be added without
changing the client-facing streaming flow.

## Current capabilities

- recursive scanning of MP3, FLAC, Ogg/Opus, M4A, AAC, and WAV files;
- tag extraction with directory/filename fallbacks and conservative repair of legacy Cyrillic encodings;
- a migrated SQLite metadata catalog;
- XML and JSON OpenSubsonic responses;
- password, hex-encoded password, and token/salt authentication;
- artist/album/track browsing and HTTP range streaming;
- genre browsing and album filtering from embedded tags;
- paged album lists ordered by name, artist, import time, year, or randomly;
- embedded and folder cover artwork stored through the blob storage boundary;
- cached OpenSubsonic artwork resizing for mobile clients;
- shared media cache with a size limit, LRU eviction, and pinned entries;
- `.torrent` catalog import with on-demand, seekable track streaming;
- on-demand album artwork from image files included in torrents;
- background tag enrichment after a torrent track has been streamed completely;
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
/rest/getSongsByGenre[.view]
/rest/getRandomSongs[.view]
/rest/getSong[.view]
/rest/getCoverArt[.view]
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
| `cache_size_bytes` | `PEERPHONIC_CACHE_SIZE_BYTES` | `--cache-size` | `10737418240` (10 GiB) |
| `torrent_dir` | `PEERPHONIC_TORRENT_DIR` | `--torrents` | `torrents` |
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

Cache usage is available from `GET /api/v1/cache/status`. The total includes
the shared media cache and provider-managed data such as complete and partial
torrent files. The response includes a component breakdown and counts partial
entries separately. Physical disk allocation is used for sparse partial files.
Unpinned and inactive entries are evicted by least recent access when the
combined configured size limit is exceeded.

Place `.torrent` files in the configured torrent directory and start a library
scan. Audio entries and references to included cover images appear in the catalog
without joining the swarm. Peerphonic starts its BitTorrent client on the first
track or cover request. The reader prioritizes only the requested byte range and
a small readahead window, so OpenSubsonic clients can begin playback while the
selected track is downloading. Downloaded pieces are reused from
`cache_dir/torrents` on later requests.

Torrent cache cleanup never removes data from an actively streamed torrent.
Before removing inactive torrent files, Peerphonic detaches that torrent from
the client so its piece state is safely re-evaluated the next time the track is
played.

When a client supplies the OpenSubsonic `getCoverArt` `size` parameter,
Peerphonic preserves the aspect ratio and caches the generated variant. This is
especially useful when torrent artwork is a high-resolution booklet scan.

After a track has been read completely from start to finish, Peerphonic inspects
the materialized cache file in the background and updates its title, track/disc
number, year, bitrate, duration where supported, and embedded artwork. Short
range requests and seeks do not mark a track as complete. Album identity remains
stable while individual tracks are enriched so existing client links keep working.

Torrent metadata can also be uploaded with the Peerphonic API using the same
credentials as OpenSubsonic:

```bash
curl -u admin:admin --data-binary @album.torrent \
  http://localhost:8080/api/v1/torrents
```

## Development

```bash
cd backend
go test ./...
go vet ./...
```

The `web/` directory is reserved for an optional Peerphonic-specific frontend.
The backend does not depend on it. Container deployment files live in `deploy/`.
