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
