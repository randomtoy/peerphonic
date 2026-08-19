# Operations

## Health and readiness

Use the unauthenticated process health endpoint for liveness and the readiness
endpoint for dependencies and metadata storage:

```bash
curl http://localhost:8080/api/v1/health
curl http://localhost:8080/api/v1/ready
```

Authenticated Prometheus metrics are available at `/api/v1/metrics`. Avoid
placing credentials directly in a world-readable monitoring configuration.

## Library synchronization

Startup and periodic scans are controlled independently. Disable the startup
scan with `scan_on_start=false`; disable periodic scans with
`scan_interval_seconds=0`. A manual scan can be started in the dashboard or via
the Peerphonic API:

```bash
curl -u admin:password -X POST http://localhost:8080/api/v1/library/scan
curl -u admin:password http://localhost:8080/api/v1/library/scan
```

Scans run in the background. File warnings and recent maintenance outcomes are
visible in **Sources**.

## SQLite backup

Create a consistent metadata and credential-key archive without stopping the
server:

```bash
peerphonic backup \
  --database /srv/peerphonic/peerphonic.db \
  --output /srv/backups/peerphonic-$(date +%F).tar.gz
```

The archive contains a verified SQLite snapshot, the credential encryption key,
and a checksum manifest. Existing output files are not overwritten. Treat the
archive as a secret. Music, cached media, and torrent files are not included.

Restore while Peerphonic is stopped:

```bash
peerphonic restore \
  --input /srv/backups/peerphonic-2026-08-19.tar.gz \
  --database /srv/peerphonic/peerphonic.db
```

Restore refuses to overwrite an existing database. `--force` preserves the old
database and key as timestamped recovery copies before replacement.

## PostgreSQL backup

Use a normal `pg_dump` process for external PostgreSQL. The Helm chart can create
a scheduled backup job that writes a custom-format dump and credential key to
the Peerphonic data PVC. Those archives still exclude music and cache data.

Test restore procedures periodically. A backup that has never been restored is
not a verified recovery plan.

## Cache and downloads

Use **Activity** to inspect selected-track jobs and **Cache** for capacity and
pinning. API equivalents include:

```bash
curl -u admin:password http://localhost:8080/api/v1/cache/status
curl -u admin:password http://localhost:8080/api/v1/downloads
curl -u admin:password http://localhost:8080/api/v1/transfers
```

A track reported as waiting may have no reachable torrent seed or no available
Soulseek slot. A stalled transfer can recover when peers return. Permanent
provider failures can be retried or resolved through an alternative source.

## Upgrade checklist

1. Read the changes and take a metadata backup.
2. Keep the database, credential key, data PVC, and music mounts intact.
3. Replace the binary or image.
4. Wait for `/api/v1/ready` before reconnecting clients.
5. Confirm the library count and test one local and one remote track.

Database migrations run with the application. Do not edit the schema manually.

## Troubleshooting

### Login works in one client but not another

Confirm the server URL contains only the Peerphonic base address, without
`/rest`. Check whether the client expects HTTP or HTTPS and whether its configured
username matches an existing Peerphonic account.

### Directories are visible but empty

Wait for the scan to finish and review warnings in **Sources**. Verify that the
process can read the mounted music directory and recognizes the file extension.

### A remote track takes a long time to start

Review peers, seeders, queue state, and current speed in **Activity**. Remote
playback cannot be made faster than the available source. Confirm inbound ports,
VPN behavior, reverse-proxy timeouts, and cache capacity.

### Artwork is missing

Local artwork requires embedded images or a supported image in the album folder.
Remote artwork may not be available until its source is reachable and cached.
