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
without exposing the API key to the browser.

For local frontend work, serve this directory through a web server that proxies
the API paths to the backend. Opening `index.html` directly does not provide an
API proxy.
