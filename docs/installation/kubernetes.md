# Kubernetes installation

The Helm chart installs:

- one Peerphonic backend pod;
- the optional administration dashboard;
- a dedicated PostgreSQL StatefulSet and PVC by default;
- an optional slskd sidecar sharing the Peerphonic data PVC;
- Services for HTTP, BitTorrent TCP/UDP, and Soulseek TCP;
- optional Ingress and scheduled metadata backups.

The backend intentionally uses one replica and the `Recreate` strategy. Torrent
sessions, slskd, cache state, and the default `ReadWriteOnce` claim are not safe
to run as independent active replicas.

## Requirements

- Kubernetes with a default StorageClass or explicitly selected classes;
- Helm 3 or newer;
- a music PVC, a single-node `hostPath`, or an initially empty library;
- an ingress controller if using Ingress;
- reachable TCP/UDP services if accepting inbound P2P traffic.

## Create a values file

The following example disables Soulseek and mounts an existing music claim:

```yaml
auth:
  adminUsername: admin
  adminPassword: choose-a-private-password

database:
  driver: postgres
  url: postgres://peerphonic:choose-a-database-password@peerphonic-postgresql:5432/peerphonic?sslmode=disable

postgresql:
  auth:
    password: choose-a-database-password
    postgresPassword: choose-a-postgres-admin-password
  primary:
    persistence:
      storageClass: standard
      size: 10Gi

slskd:
  enabled: false

music:
  existingClaim: music-library

persistence:
  storageClass: standard
  size: 50Gi

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: music.example.com
      paths:
        - path: /
          pathType: Prefix
```

Save it outside the repository as `peerphonic-values.yaml`. Replace the storage
class, host, claim, and every password. For production, create Kubernetes
Secrets instead of storing credentials in values. The supported existing-secret
names and key mappings are documented in the chart's `values.yaml`.

## Install from the repository

```bash
helm upgrade --install peerphonic deploy/helm/peerphonic \
  --namespace peerphonic \
  --create-namespace \
  --values peerphonic-values.yaml \
  --wait \
  --timeout 10m
```

Check the deployment:

```bash
kubectl -n peerphonic get pods,services,ingress,pvc
kubectl -n peerphonic rollout status deployment/peerphonic
kubectl -n peerphonic rollout status deployment/peerphonic-web
kubectl -n peerphonic exec deployment/peerphonic -c peerphonic -- \
  wget -qO- http://127.0.0.1:8080/api/v1/ready
```

Published releases can also be installed from the OCI chart repository. Choose
an existing release version instead of copying the example version literally:

```bash
helm upgrade --install peerphonic \
  oci://ghcr.io/randomtoy/charts/peerphonic \
  --version VERSION \
  --namespace peerphonic \
  --create-namespace \
  --values peerphonic-values.yaml
```

## Enable bundled slskd

slskd runs as a sidecar so it and Peerphonic see growing download files on the
same PVC. Add the following values and use a real Soulseek account:

```yaml
slskd:
  enabled: true
  apiKey: replace-with-at-least-16-random-characters
  webUsername: slskd
  webPassword: choose-a-private-web-password
  soulseekUsername: your-soulseek-username
  soulseekPassword: your-soulseek-password
```

The slskd HTTP Service is `ClusterIP`. For maintenance, open a temporary local
tunnel instead of exposing its API publicly:

```bash
kubectl -n peerphonic port-forward service/peerphonic-slskd 5030:5030
```

## Music storage

The portable option is a read-only PVC:

```yaml
music:
  existingClaim: music-library
  readOnly: true
```

For a single-node home cluster, `music.hostPath` can reference a node directory.
It ties the pod to data available on that node and is not suitable for arbitrary
rescheduling. If neither a claim nor host path is provided, `/music` is empty
ephemeral storage.

The Peerphonic data claim stores its credential key, torrent metadata, cache,
Soulseek state, partial downloads, and completed downloads. PostgreSQL uses a
separate claim.

## Ingress and P2P networking

The dashboard Ingress also routes `/api` and `/rest`, so OpenSubsonic clients can
use the same hostname. Terminate TLS at the ingress controller and do not expose
the service over plain HTTP on an untrusted network.

BitTorrent needs TCP and UDP, while Soulseek needs TCP. In clusters with a load
balancer, use:

```yaml
peerphonic:
  torrentService:
    type: LoadBalancer

slskd:
  networkService:
    type: LoadBalancer
```

`NodePort` is also supported when the corresponding node and router ports are
forwarded explicitly. UPnP from inside Kubernetes is usually unreliable because
the pod does not own the node's LAN address.

## Backups and upgrades

Enable scheduled metadata backups on the data PVC:

```yaml
backup:
  enabled: true
  schedule: "0 3 * * *"
```

PostgreSQL backups use `pg_dump`; SQLite deployments use Peerphonic's snapshot
command. Archives contain metadata and the credential key, not music or cached
media. Apply retention using the storage platform or a reviewed cleanup policy.

Upgrade with the same values file and a newer chart or image version. Review
release notes, take a backup, and keep `--wait` enabled so Helm reports failed
rollouts.
