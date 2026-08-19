# OpenSubsonic compatibility

OpenSubsonic is an external compatibility adapter. Peerphonic-specific
administration endpoints use `/api/v1/*`; music clients use `/rest/*`.

## Authentication

Peerphonic supports plain password, hex-encoded password, and token/salt
authentication. XML and JSON response formats are available. Use HTTPS whenever
the server is reachable over an untrusted network.

## Implemented endpoints

```text
/rest/ping[.view]
/rest/getLicense[.view]
/rest/getMusicFolders[.view]
/rest/getIndexes[.view]
/rest/getMusicDirectory[.view]
/rest/getGenres[.view]
/rest/getArtists[.view]
/rest/getArtist[.view]
/rest/getAlbumList[.view]
/rest/getAlbumList2[.view]
/rest/getAlbum[.view]
/rest/getSongsByGenre[.view]
/rest/getRandomSongs[.view]
/rest/getSong[.view]
/rest/getCoverArt[.view]
/rest/search2[.view]
/rest/search3[.view]
/rest/getPlaylists[.view]
/rest/getPlaylist[.view]
/rest/createPlaylist[.view]
/rest/updatePlaylist[.view]
/rest/deletePlaylist[.view]
/rest/star[.view]
/rest/unstar[.view]
/rest/setRating[.view]
/rest/scrobble[.view]
/rest/getStarred[.view]
/rest/getStarred2[.view]
/rest/getPlayQueue[.view]
/rest/savePlayQueue[.view]
/rest/getOpenSubsonicExtensions[.view]
/rest/getScanStatus[.view]
/rest/startScan[.view]
/rest/stream[.view]
/rest/download[.view]
```

This is not a promise of complete Subsonic compatibility. Clients can rely on
extensions or response quirks outside this list.

## Streaming and range requests

The stream endpoint resolves local, cached, torrent, or Soulseek media through
the same application service. It supports HTTP range requests after a source can
provide the requested bytes. Remote sources may need a prebuffer and can fail
when their peers disappear.

Reverse proxies should disable response buffering for `/rest/stream` and
`/rest/download` and allow long-lived upstream responses. The bundled web nginx
and Helm configuration already do this.

## Transcoding

When FFmpeg is available, a request with `format=mp3` is transcoded while
streaming. A completely read result is cached and reused with normal range and
content-length behavior. Set `ffmpeg_path` to an empty value to disable
transcoding and return original formats.

## Search behavior

Catalog results are returned first. Users with `soulseek.client-search` can also
receive temporary Soulseek results in remaining song slots. Starting playback
persists ranked physical sources and enters the regular buffered download path.
A Soulseek outage does not make local search fail.

## Smoke test

Run the basic discovery flow against a live server:

```bash
PEERPHONIC_URL=http://localhost:8080 \
PEERPHONIC_USERNAME=admin \
PEERPHONIC_PASSWORD=password \
backend/scripts/opensubsonic-smoke.sh
```

Set `PEERPHONIC_TRACK_ID` to include a byte-range streaming check.
