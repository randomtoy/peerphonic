# Native installation

Use a native installation when you want a single Peerphonic process with SQLite
and do not need the bundled web proxy or PostgreSQL.

## Requirements

- Go 1.25 or newer;
- a readable music directory;
- FFmpeg in `PATH` if clients need MP3 transcoding;
- optional: a separately installed slskd instance for Soulseek.

## Build

```bash
git clone https://github.com/randomtoy/peerphonic.git
cd peerphonic/backend
go build -o peerphonic ./cmd/peerphonic
```

## First start

Choose a private bootstrap password before creating the database:

```bash
PEERPHONIC_USERNAME=admin \
PEERPHONIC_PASSWORD='choose-a-private-password' \
./peerphonic serve \
  --music /absolute/path/to/Music \
  --database /absolute/path/to/peerphonic.db \
  --cache /absolute/path/to/peerphonic-cache \
  --torrents /absolute/path/to/torrents
```

The backend listens on `:8080`. Connect an OpenSubsonic client to
`http://localhost:8080`, or verify it directly:

```bash
curl 'http://localhost:8080/rest/ping?u=admin&p=choose-a-private-password&v=1.16.1&c=curl&f=json'
```

The configured username and password are bootstrap values. Once the database
contains users, changing those startup values does not overwrite existing
accounts. Use the dashboard or Peerphonic API to manage them.

## Run from a JSON configuration

Copy and edit the provided example:

```bash
cp config.example.json peerphonic.json
./peerphonic serve --config peerphonic.json
```

Configuration precedence is: defaults, JSON file, environment variables, then
CLI flags. See the [configuration reference](../configuration.md).

## Run as a service

Use the service manager supplied by your operating system. Give the service
account read access to the music directory and read/write access to the database,
credential key, cache, and torrent directories. Stop the process gracefully with
`SIGTERM` so active HTTP requests receive a shutdown window.

Back up both the SQLite database and its generated `<database>.auth.key` file.
The supported online backup command is described in [Operations](../operations.md).

## Optional dashboard

The dashboard in `web/` is static but expects `/api/` and `/rest/` to be proxied
to the backend. Docker Compose already supplies this proxy. For a native setup,
serve the directory through nginx or another web server and proxy both prefixes
to port `8080`; opening `index.html` directly is insufficient.
