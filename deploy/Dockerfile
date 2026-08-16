FROM golang:1.25-alpine AS build

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/peerphonic ./cmd/peerphonic

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/peerphonic /peerphonic
VOLUME ["/data", "/music"]
EXPOSE 8080
ENTRYPOINT ["/peerphonic", "serve"]
CMD ["--music", "/music", "--database", "/data/peerphonic.db", "--cache", "/data/cache"]
