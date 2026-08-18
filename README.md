# Peerphonic

Peerphonic is a self-hosted music server with OpenSubsonic compatibility. The
current milestone serves a local music library. Its core is designed so cache,
self-hosted storage, friends, Soulseek, and other P2P sources can be added without
changing the client-facing streaming flow.

## Current capabilities

- recursive scanning of MP3, FLAC, Ogg/Opus, M4A, AAC, and WAV files;
- periodic background synchronization of local music and torrent metadata;
- tag extraction with directory/filename fallbacks and conservative repair of legacy Cyrillic encodings;
- a migrated SQLite metadata catalog;
- XML and JSON OpenSubsonic responses;
- password, hex-encoded password, and token/salt authentication;
- persistent administrator and listener accounts with isolated personal library state;
- per-user dashboard capabilities delegated by administrators;
- artist/album/track browsing and HTTP range streaming;
- genre browsing and album filtering from embedded tags;
- paged album lists ordered by name, artist, import time, year, or randomly;
- persistent ordered playlists through the OpenSubsonic API;
- embedded and folder cover artwork stored through the blob storage boundary;
- cached OpenSubsonic artwork resizing for mobile clients;
- shared media cache with a size limit, LRU eviction, and pinned entries;
- `.torrent` catalog import with on-demand, seekable track streaming;
- persistent background completion of a selected torrent track after its first playback request;
- on-demand album artwork from image files included in torrents;
- background tag enrichment after a torrent track has been streamed completely;
- persistent favorites, ratings, and playback history per OpenSubsonic user;
- persistent cross-client OpenSubsonic play queues;
- authenticated live source transfer status for download, upload, peer, and seeding visibility;
- authenticated torrent source management with persistent pause and cache pinning;
- optional authenticated Soulseek search through a slskd source-provider adapter;
- an optional containerized administration dashboard for cache, transfers, and torrent sources;
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
/rest/getAlbumList[.view]
/rest/getAlbumList2[.view]
/rest/getAlbum[.view]
/rest/getSongsByGenre[.view]
/rest/getRandomSongs[.view]
/rest/getSong[.view]
/rest/getCoverArt[.view]
/rest/search2[.view]
/rest/search3[.view]
/rest/getPlaylists[.view]
/rest/getPlaylist[.view]
/rest/createPlaylist[.view]
/rest/updatePlaylist[.view]
/rest/deletePlaylist[.view]
/rest/star[.view]
/rest/unstar[.view]
/rest/setRating[.view]
/rest/scrobble[.view]
/rest/getStarred[.view]
/rest/getStarred2[.view]
/rest/getPlayQueue[.view]
/rest/savePlayQueue[.view]
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

The server listens on `:8080`, stores its catalog in `peerphonic.db`, and creates
`admin` / `admin` when the user table is empty. Set a private bootstrap password
before the first start or change it immediately in the administration dashboard:

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

The configured username and password are bootstrap values only. Once at least
one account exists, changing the environment or CLI values does not overwrite
database users. Administrators can create listener or administrator accounts,
reset passwords, and remove accounts from the dashboard. User records contain a
bcrypt password hash. OpenSubsonic token authentication additionally requires a
reversible credential protected by a random AES-GCM key stored next to the
database as `<database>.auth.key`; back up that `0600` file together with the
database.

Administrators always have every management capability. Regular users can be
granted these independently:

| Capability | Access |
| --- | --- |
| `dashboard.access` | Sign in to the administration dashboard |
| `monitoring.view` | View cache usage, selected-track downloads, peers, and transfers |
| `sources.manage` | Add, pause, pin, and remove torrent sources; scan the library; change torrent limits |
| `users.manage` | Create, reset, and remove regular user accounts |

Only administrators can assign capabilities, create administrators, or manage
administrator accounts. A delegated user manager cannot elevate itself or
another user.

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
| `torrent_seed` | `PEERPHONIC_TORRENT_SEED` | `--torrent-seed` | `true` |
| `torrent_port` | `PEERPHONIC_TORRENT_PORT` | `--torrent-port` | `42069` |
| `torrent_port_forwarding` | `PEERPHONIC_TORRENT_PORT_FORWARDING` | `--torrent-port-forwarding` | `false` |
| `torrent_upload_limit_bytes_per_second` | `PEERPHONIC_TORRENT_UPLOAD_LIMIT_BYTES_PER_SECOND` | `--torrent-upload-limit` | `0` (unlimited) |
| `torrent_download_limit_bytes_per_second` | `PEERPHONIC_TORRENT_DOWNLOAD_LIMIT_BYTES_PER_SECOND` | `--torrent-download-limit` | `0` (unlimited) |
| `username` | `PEERPHONIC_USERNAME` | `--username` | `admin` |
| `password` | `PEERPHONIC_PASSWORD` | `--password` | `admin` |
| `scan_on_start` | `PEERPHONIC_SCAN_ON_START` | `--scan` | `true` |
| `scan_interval_seconds` | `PEERPHONIC_SCAN_INTERVAL_SECONDS` | `--scan-interval` | `300` |
| `slskd_url` | `PEERPHONIC_SLSKD_URL` | `--slskd-url` | disabled |
| `slskd_api_key` | `PEERPHONIC_SLSKD_API_KEY` | — | empty |
| `slskd_timeout_seconds` | `PEERPHONIC_SLSKD_TIMEOUT_SECONDS` | `--slskd-timeout` | `5` |
| `slskd_downloads_dir` | `PEERPHONIC_SLSKD_DOWNLOADS_DIR` | `--slskd-downloads` | `<cache>/soulseek/downloads` |
| `slskd_incomplete_dir` | `PEERPHONIC_SLSKD_INCOMPLETE_DIR` | `--slskd-incomplete` | `<cache>/soulseek/incomplete` |

Use a config file with `--config peerphonic.json` or set its path through
`PEERPHONIC_CONFIG`. See [`backend/config.example.json`](backend/config.example.json).
The periodic scan runs independently of `scan_on_start`; set
`scan_interval_seconds` to `0` when only manual OpenSubsonic `startScan` calls
should update the catalog.

Upload and download limits saved through the Peerphonic API or dashboard are
stored in SQLite and override their startup configuration values on subsequent
runs. Saving `0` restores unlimited transfer speed.

User administration is also available through the Peerphonic API:

```bash
curl -u admin:admin http://localhost:8080/api/v1/users
curl -u admin:admin -H 'Content-Type: application/json' \
  -d '{"username":"listener","password":"replace-this-password","role":"user"}' \
  http://localhost:8080/api/v1/users
curl -u admin:admin -X PUT -H 'Content-Type: application/json' \
  -d '{"permissions":["dashboard.access","monitoring.view"]}' \
  http://localhost:8080/api/v1/users/listener/permissions
curl -u admin:admin http://localhost:8080/api/v1/library/scan
curl -u admin:admin -X POST http://localhost:8080/api/v1/library/scan
curl -u admin:admin http://localhost:8080/api/v1/settings/transfers
curl -u admin:admin -X PUT -H 'Content-Type: application/json' \
  -d '{"downloadLimitBytesPerSecond":10485760,"uploadLimitBytesPerSecond":2097152}' \
  http://localhost:8080/api/v1/settings/transfers
curl -u admin:admin http://localhost:8080/api/v1/providers/soulseek/status
curl -u admin:admin -X POST -H 'Content-Type: application/json' \
  -d '{"query":"Massive Attack Mezzanine","limit":50}' \
  http://localhost:8080/api/v1/providers/soulseek/search
curl -u admin:admin -X POST \
  http://localhost:8080/api/v1/providers/soulseek/tracks/SEARCH_RESULT_ID
```

Library scans started through the Peerphonic API run in the background. Their
status includes the indexed track count and the last start, completion, or
error; the same controls are available in the dashboard Sources workspace.

To prepare Soulseek integration, configure the slskd base URL and an API key
with read/write access. Peerphonic authenticates with the `X-API-Key` header and
uses slskd's `/api/v0/session` endpoint for readiness checks. Prefer HTTPS or a
private container/Kubernetes network because the API key is a long-lived
secret. Peerphonic can search the Soulseek network, add a selected result to the
catalog without downloading it, and enqueue only that file when an OpenSubsonic
client starts playback. Reads follow slskd's growing incomplete file so clients
can buffer before the complete download has finished. Completed files remain in
the downloads directory and are reused by later playback requests.

Peerphonic and slskd must see the same completed and incomplete storage. For a
native installation, configure slskd's `SLSKD_DOWNLOADS_DIR` and
`SLSKD_INCOMPLETE_DIR` to the two Peerphonic directories above. In containers or
Kubernetes, mount the same volume into both services; the absolute mount paths
may differ, but each pair must point at the same directory on that volume.

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
`cache_dir/torrents/<info-hash>` on later requests. Legacy cache files are moved
into the matching info-hash directory as their torrent metadata is scanned, so
different torrents with identical internal paths cannot overwrite each other.

Verified downloaded pieces are uploaded to other peers by default while the
torrent remains attached. Set `torrent_seed` to `false` to disable seeding. A
stable listen port makes manual router forwarding possible; alternatively,
enable `torrent_port_forwarding` to let Peerphonic request UPnP/NAT-PMP mapping.
Upload and download limits are independent byte-per-second ceilings. A value of
zero leaves that direction unlimited; limits apply to aggregate torrent traffic,
not separately to each peer or track.

Live transfer state is available without attaching inactive catalog sources:

```bash
curl -u admin:admin http://localhost:8080/api/v1/transfers
```

The response reports persistent completed bytes and current-process download
and upload counters, peers, active streams, and seeding state for each attached
source. An empty list means no source has been opened since server startup.

The first playback request for a torrent track also creates a persistent
single-track cache job. It continues after the OpenSubsonic HTTP stream closes,
does not request other files in the torrent, and is resumed after a server
restart. Per-track progress and cached/failed/evicted state are available at:

```bash
curl -u admin:admin http://localhost:8080/api/v1/downloads
```

These jobs are also shown in the administration dashboard. Completed pieces
remain available for upload while the torrent is attached and seeding is
enabled.

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

Magnet links are accepted asynchronously so slow metadata discovery does not
hold an HTTP connection open:

```bash
curl -u admin:admin -H 'Content-Type: application/json' \
  -d '{"magnet":"magnet:?xt=urn:btih:..."}' \
  http://localhost:8080/api/v1/torrents/magnet
curl -u admin:admin http://localhost:8080/api/v1/imports
```

The import endpoint returns HTTP 202 immediately. Jobs move through
`fetching_metadata`, `scanning`, and `ready` or `failed`; unfinished jobs resume
after a server restart. Fetching magnet metadata does not select media files,
so track data still starts on the first playback request. Both magnet links and
`.torrent` files can also be added from the administration dashboard.

Imported torrents can be listed and managed without restarting the server:

```bash
curl -u admin:admin http://localhost:8080/api/v1/torrents
curl -u admin:admin -X POST http://localhost:8080/api/v1/torrents/INFO_HASH/pause
curl -u admin:admin -X POST http://localhost:8080/api/v1/torrents/INFO_HASH/resume
curl -u admin:admin -X POST http://localhost:8080/api/v1/torrents/INFO_HASH/pin
curl -u admin:admin -X POST http://localhost:8080/api/v1/torrents/INFO_HASH/unpin
curl -u admin:admin -X DELETE \
  'http://localhost:8080/api/v1/torrents/INFO_HASH?deleteData=true'
```

Pause state and whole-torrent cache pins survive restarts. A pinned source is
excluded from automatic cache eviction. Removing a source always removes its
catalog metadata and triggers a library synchronization; `deleteData=true` also
removes its cached media. Active streams return HTTP 409 instead of being
interrupted.

## Development

```bash
cd backend
go test ./...
go vet ./...
```

The static administration dashboard lives in `web/`; the backend does not
depend on it. The container deployment serves the dashboard on port `8081` and
the OpenSubsonic backend on port `8080`. See `deploy/README.md` for startup
instructions.
