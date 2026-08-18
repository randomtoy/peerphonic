# Peerphonic Helm chart

This chart installs Peerphonic, its administration dashboard, and optionally
slskd. Peerphonic and slskd run in the same pod and mount the same data volume.
This sidecar layout guarantees that growing Soulseek downloads are immediately
visible to Peerphonic and keeps the slskd API off the public network.

The backend deployment intentionally uses one replica and the `Recreate`
strategy. SQLite and slskd are single-writer components; horizontal backend
scaling requires a different metadata and provider topology.

## Images

Version tags publish the backend and web images to GitHub Container Registry.
The packaged chart selects the matching image tag from its `appVersion`. For
local development, build both images and override their repositories and tags.
The official slskd image is used by default.

```bash
docker build -f deploy/Dockerfile -t registry.example/peerphonic:latest .
docker build -f web/Dockerfile -t registry.example/peerphonic-web:latest .
docker push registry.example/peerphonic:latest
docker push registry.example/peerphonic-web:latest
```

## Installation

Create a values file that is not committed to source control:

```yaml
auth:
  adminPassword: replace-this-password

slskd:
  apiKey: replace-with-at-least-16-characters
  webPassword: replace-this-password
  soulseekUsername: your-soulseek-account
  soulseekPassword: your-soulseek-password

music:
  existingClaim: music-library

persistence:
  size: 50Gi
```

Install the chart:

```bash
helm upgrade --install peerphonic deploy/helm/peerphonic \
  --namespace peerphonic --create-namespace \
  -f peerphonic-values.yaml
```

Published chart versions can also be installed directly from GHCR:

```bash
helm upgrade --install peerphonic \
  oci://ghcr.io/randomtoy/charts/peerphonic \
  --version 0.1.0 \
  --namespace peerphonic --create-namespace \
  -f peerphonic-values.yaml
```

Creating and pushing a semantic version tag runs the release workflow:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Instead of placing credentials in a values file, create a Secret and set
`auth.existingSecret`. Its key names are configurable under `auth.keys`.

If neither `music.existingClaim` nor `music.hostPath` is set, `/music` is an
empty ephemeral directory. `hostPath` is convenient for a single-node home
cluster; a read-only PVC is more portable.

## Network access

The dashboard Ingress proxies both `/api` and `/rest`, so the same hostname can
be used by a browser and OpenSubsonic clients. slskd's HTTP service remains
`ClusterIP`; reach its UI for maintenance with:

```bash
kubectl -n peerphonic port-forward service/peerphonic-slskd 5030:5030
```

BitTorrent uses TCP and UDP port `42069`. Soulseek uses TCP port `50300`.
Their services default to `ClusterIP`, which is safe but does not accept inbound
internet connections. On a cluster with a load balancer, set:

```yaml
peerphonic:
  torrentService:
    type: LoadBalancer

slskd:
  networkService:
    type: LoadBalancer
```

For `NodePort`, specify a valid `nodePort` and forward that external port to the
node. The Soulseek listen port advertised by slskd must match the externally
reachable port. UPnP inside Kubernetes is generally unreliable because the pod
does not own the node's LAN address; use a Service or explicit router forwarding.

## Storage

The data claim contains SQLite, torrent metadata and cache, Soulseek application
state, completed files, and partial files. Peerphonic and slskd use the same
paths under `/data/cache/soulseek`, which enables progressive playback while a
file is still downloading.

Use `persistence.existingClaim` to retain an already provisioned PVC. Set
`persistence.enabled=false` only for disposable testing.
