# Peerphonic Web

This directory is reserved for an optional Peerphonic-specific web application.
It will expose source availability, cache state, downloads, peer status, and
provider configuration in later milestones.

The backend already exposes authenticated live transfer data at
`GET /api/v1/transfers`; a future frontend can consume it without depending on
the concrete torrent adapter.

The backend remains independently usable by OpenSubsonic clients. No frontend
toolchain or runtime dependency is introduced in the local-library milestone.
