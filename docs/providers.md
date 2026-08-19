# Music sources and caching

Peerphonic exposes one logical catalog while resolving media through different
providers. The OpenSubsonic API does not contain provider-specific behavior.

## Local files

The local provider scans the configured music directory recursively. Files are
never rewritten. Embedded tags and artwork are read into the metadata catalog;
folder artwork is stored through the media storage boundary. Conservative repair
handles some legacy Cyrillic metadata encodings.

Local tracks stream in their original format unless the client requests a format
that requires FFmpeg transcoding. The source directory should normally be
mounted read-only.

## BitTorrent

Place `.torrent` metadata in the configured torrent directory, upload it through
the dashboard, or add a magnet link. A catalog scan reads audio paths and cover
images without downloading the release.

On first playback, Peerphonic joins the torrent and prioritizes the byte range
needed by the stream plus a small readahead window. Verified pieces are stored
under `cache/torrents/<info-hash>` and reused later. A persistent selected-track
job can continue after the HTTP stream closes; it does not select unrelated
files in the release.

Verified pieces are uploaded to peers by default while the torrent remains
attached. Set `torrent_seed` to `false` to disable seeding. Upload and download
limits apply to aggregate torrent traffic. A value of zero means unlimited.

Availability is not guaranteed. A torrent with no reachable seed may stay
waiting indefinitely. Stable inbound TCP/UDP connectivity usually improves
peer discovery and upload participation.

## Soulseek through slskd

Soulseek is implemented through a separately authenticated slskd API. Peerphonic
can search, preview a remote directory, add track or album metadata, enqueue one
file on demand, read a growing incomplete file, and reuse completed downloads.

Peerphonic and slskd must share completed and incomplete storage. Their absolute
paths can differ only when both map to the same underlying directories. The Helm
chart avoids this mismatch by running slskd as a sidecar with the same PVC.

Peer copies can disappear, reject a transfer, or change size. Peerphonic records
ranked alternatives and can fall back after a stale source fails. Search and
playback still depend on the Soulseek network and the remote user's availability.

## Logical identities

Tracks from local files, torrents, and Soulseek can represent the same recording.
Peerphonic uses provider-independent artist, album, and logical track identities
to avoid exposing separate copies to clients where metadata is sufficiently
similar. Administrators can correct remaining artist duplicates with reversible
catalog aliases.

## Shared cache

The configured cache limit covers reusable media plus provider-managed complete
and partial data. Peerphonic reports physical disk allocation for sparse torrent
files and tracks provider components separately.

Unpinned, inactive entries are evicted by least recent access when the combined
limit is exceeded. Active streams are protected. Pinning a track, album, or
torrent source prevents its completed provider data from automatic eviction.

When FFmpeg produces a complete transcoded variant, the result joins the same
cache and supports later content-length and HTTP range requests. Concurrent
requests for the same variant share one transcoding operation.

## Artwork and metadata enrichment

Embedded, folder, torrent, and Soulseek artwork is cached on first use. Requested
thumbnail sizes preserve aspect ratio and are cached as variants.

After a remote track is completely materialized, Peerphonic reads its tags and
embedded artwork in the background. Provisional album identity remains stable so
existing client links do not break during enrichment.
