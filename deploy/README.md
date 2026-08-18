# Container deployment

Set the host music directory and a password, then start the service:

```bash
cd deploy
export MUSIC_DIR=/path/to/Music
export PEERPHONIC_PASSWORD='replace-this-password'
docker compose up --build -d
```

Catalog and cache data are stored in `deploy/data/`. The music directory
is mounted read-only at `/music`. OpenSubsonic remains available on port `8080`;
the optional administration dashboard is served at `http://localhost:8081`.
Set `PEERPHONIC_WEB_PORT` to publish the dashboard on another host port.

The compose service publishes the BitTorrent listen port over both TCP and UDP.
It defaults to `42069`; override `PEERPHONIC_TORRENT_PORT` before startup when
the host port is already occupied. Forward the same TCP/UDP port on the router
for inbound peers when automatic port forwarding is disabled.

An existing slskd instance can be connected by setting
`PEERPHONIC_SLSKD_URL` and `PEERPHONIC_SLSKD_API_KEY` before starting Compose.
When both services run in the same Compose or Kubernetes network, use the
internal service URL rather than publishing the slskd API to the internet.
For playback, slskd must mount `deploy/data/cache/soulseek` and use its
`downloads` and `incomplete` children as `SLSKD_DOWNLOADS_DIR` and
`SLSKD_INCOMPLETE_DIR`. Peerphonic reads incomplete files from that shared
storage while they grow and reuses completed downloads as its Soulseek cache.

For Kubernetes, use the chart in `deploy/helm/peerphonic`. It runs slskd as a
sidecar of the Peerphonic backend so both processes always mount the same PVC.
The chart also creates separate configurable Services for BitTorrent TCP/UDP
and the inbound Soulseek TCP port; see the chart README for ingress, secrets,
storage, and port-forwarding examples.
