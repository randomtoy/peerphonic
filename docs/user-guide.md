# User guide

## Connect a music client

Use the OpenSubsonic endpoint exposed by your installation:

```text
Server:   https://music.example.com
Username: your Peerphonic account
Password: your Peerphonic password
```

For Docker Compose without TLS, the local address is `http://localhost:8080`.
Remote access should use HTTPS through a trusted reverse proxy or ingress.

Peerphonic supports password and token/salt authentication. A client should not
need to know whether a track is local, cached, from a torrent, or from Soulseek.

## First library scan

The server scans the configured music directory at startup by default and then
every five minutes. The **Sources** workspace shows the current scan, indexed
track count, recent maintenance runs, and file warnings. Administrators and
authorized source managers can start a scan manually.

Supported local formats include MP3, FLAC, Ogg/Vorbis, Opus, AAC, M4A/ALAC, WAV,
AIFF, WMA, APE, WavPack, and Musepack. Metadata comes from embedded tags when
available, with directory and filename fallbacks.

## Dashboard workspaces

- **Overview** summarizes the library, providers, cache, and current activity.
- **Sources** scans local music and manages torrent files, magnets, and provider
  status.
- **Search** queries Soulseek when slskd is enabled and the account has access.
- **Activity** shows queued, downloading, stalled, failed, cached, and seeding
  work across remote providers.
- **Cache** shows retained media and supports track or album prefetch and pinning.
- **Catalog** manages artist aliases and targeted metadata overrides.
- **Users** creates accounts and delegates capabilities.
- **Settings** controls transfer limits and reports provider readiness.

The dashboard is an administration tool. Continue using an OpenSubsonic client
for normal browsing and playback.

## Accounts and permissions

Administrators always have every management capability. A regular user can be
granted individual capabilities:

| Capability | Effect |
| --- | --- |
| `dashboard.access` | Sign in to the dashboard |
| `monitoring.view` | View cache, downloads, peers, and transfers |
| `sources.manage` | Scan the library and manage torrents and transfer limits |
| `soulseek.search` | Search and preview Soulseek results in the dashboard |
| `soulseek.add` | Add Soulseek tracks or albums to the catalog |
| `soulseek.client-search` | Include temporary Soulseek results in client searches |
| `users.manage` | Manage regular user accounts |
| `catalog.manage` | Merge artist identities and edit track metadata |

Only administrators can assign capabilities, create administrators, or manage
administrator accounts. Delegated user managers cannot elevate themselves.

## Add a torrent source

Open **Sources**, then upload a `.torrent` file or paste a magnet link. Importing
metadata does not download every file. Peerphonic adds audio entries to the
catalog and requests a track when playback, prefetch, or pinning needs it.

Removing a source removes its catalog entries. Choose data deletion only when
the cached media is no longer needed. Active streams are protected from removal.

## Search Soulseek

Soulseek requires a reachable and authenticated slskd instance. Search results
may be added as an individual track or reviewed and added as a remote album.
Adding catalog metadata does not start the audio transfer; playback fetches the
selected file. Completed files stay in the shared cache.

Client-side Soulseek search is a separate permission. Local catalog matches are
returned first, and temporary remote matches fill remaining result slots. A
provider outage does not make local search fail.

## Download and offline buttons in clients

Peerphonic implements OpenSubsonic download endpoints for songs and collections.
An offline request may prefetch remote data before the client receives it. Large
albums and rare peers can take time; review **Activity** when a client appears to
wait. Completed cached data is reused by later playback and range requests.

## Favorites, playlists, and play queues

Favorites, ratings, playback history, playlists, and play queues are stored per
user. Multiple OpenSubsonic clients signed in as the same user share this state.
