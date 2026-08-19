# Peerphonic documentation

This documentation separates installation and operator details from the short
project overview in the repository root.

## Install Peerphonic

- [Native installation](installation/local.md) — build and run the Go backend on
  a workstation or server.
- [Docker installation](installation/docker.md) — run the backend and dashboard
  with Docker Compose or build the images directly.
- [Kubernetes installation](installation/kubernetes.md) — install the Helm chart
  with PostgreSQL, persistent storage, ingress, and optional slskd.

## Use and operate Peerphonic

- [User guide](user-guide.md) — accounts, clients, scans, dashboard workspaces,
  and common workflows.
- [Music sources and caching](providers.md) — local files, torrents, Soulseek,
  on-demand playback, and source availability.
- [Operations](operations.md) — health checks, backups, restores, cache behavior,
  upgrades, and troubleshooting.
- [Configuration reference](configuration.md) — JSON, environment variables,
  and command-line flags.
- [OpenSubsonic compatibility](opensubsonic.md) — authentication, endpoint
  coverage, transcoding, and client notes.

## Understand or contribute

- [Architecture](architecture.md) — core boundaries, adapters, storage, and
  provider-independent streaming.
- [Development](development.md) — repository layout, tests, image builds, and
  contribution workflow.
