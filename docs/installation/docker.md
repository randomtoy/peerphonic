# Docker installation

Docker Compose is the simplest installation that includes both the Peerphonic
backend and its administration dashboard.

## Requirements

- Docker Engine or Docker Desktop with Compose;
- an absolute path to a music directory;
- enough local disk space for metadata and the media cache.

## Start with Compose

```bash
git clone https://github.com/randomtoy/peerphonic.git
cd peerphonic

export MUSIC_DIR=/absolute/path/to/Music
export PEERPHONIC_USERNAME=admin
export PEERPHONIC_PASSWORD='choose-a-private-password'

docker compose -f deploy/compose.yaml up --build -d
```

The services are available at:

| Service | Address |
| --- | --- |
| OpenSubsonic and Peerphonic API | `http://localhost:8080` |
| Administration dashboard | `http://localhost:8081` |
| BitTorrent TCP and UDP | `42069` |

Set `PEERPHONIC_WEB_PORT` or `PEERPHONIC_TORRENT_PORT` before startup to change
the published dashboard or torrent port.

## Storage

Compose mounts the host music directory read-only at `/music`. Mutable data is
kept under `deploy/data/`:

```text
deploy/data/
├── peerphonic.db
├── peerphonic.auth.key
├── cache/
└── torrents/
```

Do not delete this directory during a normal upgrade. Back it up according to
[Operations](../operations.md). Cached remote media can be recreated, but the
database and credential key should be treated as persistent state.

## Manage the deployment

```bash
docker compose -f deploy/compose.yaml ps
docker compose -f deploy/compose.yaml logs -f
docker compose -f deploy/compose.yaml restart
docker compose -f deploy/compose.yaml down
```

After pulling changes, rebuild and recreate the services:

```bash
git pull --ff-only
docker compose -f deploy/compose.yaml up --build -d
```

## Build images directly

Both Dockerfiles use the repository root as their build context:

```bash
docker build -f deploy/Dockerfile -t peerphonic:local .
docker build -f web/Dockerfile -t peerphonic-web:local .
```

The backend image exposes port `8080`; the web image also listens internally on
`8080` and expects the backend to be resolvable as `peerphonic:8080`.

## Networking for torrents

Compose publishes the selected BitTorrent port over TCP and UDP. Forward the
same port from the router to the Docker host when inbound peers are required.
Native UPnP/NAT-PMP can be enabled with
`PEERPHONIC_TORRENT_PORT_FORWARDING=true`, but manual forwarding is usually more
predictable on servers.

## Optional slskd

The Compose file does not create a Soulseek account or bundled slskd service.
Connect an existing instance with:

```bash
export PEERPHONIC_SLSKD_URL=http://slskd:5030
export PEERPHONIC_SLSKD_API_KEY='a-long-private-api-key'
```

Peerphonic and slskd must see the same completed and incomplete directories.
Mount `deploy/data/cache/soulseek` into slskd and configure its `downloads` and
`incomplete` subdirectories accordingly. Keep the slskd API on a private
container network whenever possible. See [Music sources](../providers.md).
