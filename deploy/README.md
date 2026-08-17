# Container deployment

Set the host music directory and a password, then start the service:

```bash
cd deploy
export MUSIC_DIR=/path/to/Music
export PEERPHONIC_PASSWORD='replace-this-password'
docker compose up --build -d
```

Catalog and cache data are stored in `deploy/data/`. The music directory
is mounted read-only at `/music`.
