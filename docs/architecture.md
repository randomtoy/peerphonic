# Architecture

Peerphonic uses hexagonal architecture so catalog and streaming behavior do not
depend on a particular database, filesystem, source network, or HTTP client.

```text
OpenSubsonic API       Peerphonic API
        \                 /
         application services
                  |
          domain and core ports
          /       |        \
 metadata      media       source providers
 storage       storage     local / torrent / Soulseek
```

Dependencies point inward. Core packages do not import SQLite, PostgreSQL, HTTP,
filesystem, Soulseek, or BitTorrent implementations.

## Core

The domain models logical artists, albums, tracks, physical source references,
users, cache state, and transfer state. Application services coordinate catalog
queries, source resolution, streaming, caching, playlists, annotations, and user
management through small ports.

Interfaces live near their consumers. A port is introduced at a real
infrastructure boundary, not merely for theoretical abstraction.

## Metadata storage

SQLite and PostgreSQL implement the same catalog-oriented ports. Migrations are
embedded separately for each database. Persistence rows are kept outside the
domain when representation differs.

Metadata storage includes the catalog, users, playlists, annotations, source
references, transfer settings, pin intent, audit records, and cache metadata.

## Media storage

Binary media is a separate concern from metadata. Local filesystem adapters hold
artwork, cached transcodes, remote audio, and temporary provider data. Core code
depends on blob and resolved-source ports rather than concrete paths.

## Source providers

Local files, torrents, and Soulseek implement provider boundaries. The streaming
service resolves a logical track to an available physical source and returns a
seekable stream. OpenSubsonic handlers never join swarms or call slskd directly.

Provider logic is separate from transport concerns such as peer discovery,
connections, piece transfer, and upload accounting. This keeps future providers
from spreading protocol-specific concepts through the core.

## APIs

`/rest/*` is the OpenSubsonic compatibility adapter. `/api/v1/*` exposes
Peerphonic-specific administration, source management, monitoring, and health.
The static web application consumes only the latter and is optional; third-party
music clients can use the backend without it.

## Deployment constraints

PostgreSQL removes the SQLite single-writer constraint, but Peerphonic still
runs one active backend replica because torrent sessions, slskd sidecar state,
local cache coordination, and `ReadWriteOnce` storage are stateful. Horizontal
replication requires explicit distributed ownership and is not currently
implemented.
