# Peerphonic Web

This directory contains the optional Peerphonic administration dashboard. It is
a static application and has no build-time dependency on the Go backend.

The container deployment serves it at `http://localhost:8081` and proxies
`/api/` and `/rest/` to Peerphonic. Enter the same credentials configured for
OpenSubsonic clients. The dashboard shows persistent selected-track downloads,
live provider transfers, cache usage, torrent source controls, and forms for
adding magnet links or `.torrent` files. Administrators can also create users,
reset their passwords, remove accounts, and delegate individual dashboard,
monitoring, source-management, and user-management capabilities. The workspace
separates overview, source management, live activity, and user access into
permission-aware navigation areas. Users with source-management access can also
start a background library scan and follow its last result from the Sources
workspace. The Settings workspace changes aggregate torrent upload and download
limits at runtime and persists them in the metadata database. It also reports
whether an optional slskd API is configured, reachable, and authenticated
without exposing the API key to the browser. The Search workspace queries
connected Soulseek peers, shows audio files, queues, upload speeds, and free
slots, and lets an authorized user add one unlocked result or preview the track
list, duration, size, and cover availability of its remote album before importing
the directory without starting audio downloads. The search card exposes this as
an explicit “Review & add whole album” action. Matching cover artwork is
cached on demand. Search matches from the same peer and directory are grouped
into one album card, with individual track controls retained inside it. Playback later starts the single-file transfer and reuses
the completed file from the shared cache. The Activity workspace combines
torrent and Soulseek jobs, groups multiple source attempts for one logical track,
and shows sampled speed, stalled progress, connected torrent peers and seeders,
including waiting, downloading, transcoding, cached, and cancelled states.
Completed MP3 transcodes are kept in the shared LRU cache and reused for later
OpenSubsonic range requests. Accounts with
source-management access can cancel an active Soulseek transfer or retry a
failed, cancelled, or evicted one. Active Soulseek job monitoring resumes after
a Peerphonic restart without enqueueing a duplicate slskd transfer.

Soulseek access is delegated independently: `soulseek.search` exposes the Search
workspace and album previews, `soulseek.add` enables track and album imports, and
`soulseek.client-search` extends that account's OpenSubsonic searches with temporary
remote results. General source managers keep dashboard search and import access,
while music-client network search remains an explicit opt-in.

For local frontend work, serve this directory through a web server that proxies
the API paths to the backend. Opening `index.html` directly does not provide an
API proxy.
