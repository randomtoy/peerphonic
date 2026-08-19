# Development

## Repository layout

```text
peerphonic/
├── backend/
│   ├── cmd/peerphonic/
│   ├── internal/core/
│   ├── internal/adapters/
│   ├── internal/api/
│   ├── migrations/
│   └── postgresmigrations/
├── web/
├── deploy/
│   └── helm/peerphonic/
└── docs/
```

The backend follows hexagonal boundaries described in
[Architecture](architecture.md). The web dashboard is static and the backend
must remain usable without it.

## Backend checks

```bash
cd backend
go test ./...
go vet ./...
```

Add tests for meaningful behavior and focused adapter integration. Database
schema changes require migrations for both supported metadata backends where
applicable.

## Dashboard check

```bash
node --check web/app.js
```

For interactive work, serve `web/` through a proxy that forwards `/api/` and
`/rest/` to a running backend. The Docker setup provides this routing.

## Helm checks

The chart requires secrets. Existing-secret placeholders are sufficient for
render validation:

```bash
helm lint --strict deploy/helm/peerphonic \
  --set auth.existingSecret=peerphonic-auth \
  --set postgresql.auth.existingSecret=peerphonic-postgresql

helm template peerphonic deploy/helm/peerphonic \
  --set auth.existingSecret=peerphonic-auth \
  --set postgresql.auth.existingSecret=peerphonic-postgresql \
  >/tmp/peerphonic.yaml
```

## Container builds

```bash
docker build -f deploy/Dockerfile -t peerphonic:dev .
docker build -f web/Dockerfile -t peerphonic-web:dev .
```

## Branches and commits

Use a branch prefix that describes the change:

```text
feature/*
fix/*
docs/*
refactor/*
```

Keep commits small and logically complete. Do not combine unrelated changes or
leave the repository knowingly broken between completed milestones.

GitHub Actions run backend tests and vet, dashboard syntax checks, container
builds, and Helm validation. Version tags publish multi-architecture backend and
web images and the packaged Helm chart.
