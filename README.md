# Peerphonic

Peerphonic is a self-hosted music server that presents local files, cached media,
BitTorrent sources, and optional Soulseek results as one OpenSubsonic library.
Use it with compatible mobile and desktop clients such as Amperfy, Feishin, and
other Subsonic-compatible players.

Peerphonic keeps the client-facing library independent from the physical source.
A local track streams immediately, while a remote track can be fetched on demand
and reused from the server cache on later plays.

## Highlights

- local library scanning with tags, cover art, and common audio formats;
- OpenSubsonic browsing, search, playlists, favorites, play queues, and streaming;
- on-demand BitTorrent track playback with caching and optional seeding;
- optional Soulseek search and single-track downloads through slskd;
- administration dashboard for users, sources, cache, and active transfers;
- SQLite for simple installations and PostgreSQL for Kubernetes deployments;
- Docker Compose and Helm installation options.

Peerphonic is under active development. Back up its metadata before upgrades and
expect compatibility gaps with clients that depend on uncommon Subsonic extensions.

## Quick start with Docker

Requirements: Git and Docker with Compose.

```bash
git clone https://github.com/randomtoy/peerphonic.git
cd peerphonic

export MUSIC_DIR=/absolute/path/to/your/Music
export PEERPHONIC_PASSWORD='choose-a-private-password'

docker compose -f deploy/compose.yaml up --build -d
```

Open the dashboard at [http://localhost:8081](http://localhost:8081), or connect
an OpenSubsonic client with:

```text
Server:   http://localhost:8080
Username: admin
Password: the value of PEERPHONIC_PASSWORD
```

Your music directory is mounted read-only. Peerphonic stores its database,
cache, and imported torrent metadata under `deploy/data/`.

To stop the server:

```bash
docker compose -f deploy/compose.yaml down
```

See the [Docker guide](docs/installation/docker.md) for ports, storage, image
builds, and optional Soulseek integration. Native and Kubernetes installations
are documented separately.

## Everyday use

After the first login:

1. Wait for the initial local-library scan to finish in **Sources**.
2. Add the same server address and account to your OpenSubsonic client.
3. Add `.torrent` files or magnet links from the dashboard when needed.
4. Configure slskd only if you want Soulseek search and downloads.
5. Review **Activity** for remote download progress and **Cache** for retained media.

Administrators can create listener accounts and decide which dashboard and
source-management features each user may access. Remote sources are optional;
Peerphonic works as a local OpenSubsonic server without them.

## Important limitations

- Peerphonic is experimental and has not received a professional security audit.
- The backend currently runs as a single replica because cache and P2P sessions
  are stateful.
- Remote playback depends on peer availability, network reachability, and client
  timeouts. Rare sources may buffer or fail.
- BitTorrent and Soulseek inbound connectivity may require router forwarding or
  a suitable Kubernetes Service.
- OpenSubsonic support is broad but not complete; client behavior can differ.
- The web dashboard is an administration interface, not a full music player.

## Responsible use

Peerphonic is an educational and experimental software project. It does not
include music, provide a public media catalog, or grant rights to third-party
content. Only index, download, stream, or share media that you are legally
allowed to use, and follow the rules of your network providers and local law.
The project's educational purpose and software license do not remove those
responsibilities.

## Documentation

- [Documentation index](docs/README.md)
- [Native installation](docs/installation/local.md)
- [Docker installation](docs/installation/docker.md)
- [Kubernetes installation](docs/installation/kubernetes.md)
- [User guide](docs/user-guide.md)
- [Music sources and caching](docs/providers.md)
- [Operations and backups](docs/operations.md)
- [Configuration reference](docs/configuration.md)
- [OpenSubsonic compatibility](docs/opensubsonic.md)
- [Architecture](docs/architecture.md)
- [Development](docs/development.md)

## License

Peerphonic is available under the [MIT License](LICENSE). The license applies to
the software, not to media accessed with it.
