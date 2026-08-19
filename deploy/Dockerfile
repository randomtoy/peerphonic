FROM golang:1.25-alpine AS build

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/peerphonic ./cmd/peerphonic

FROM alpine:3.22

RUN apk add --no-cache ffmpeg \
    && addgroup -S -g 65532 peerphonic \
    && adduser -S -D -H -u 65532 -G peerphonic peerphonic

COPY --from=build /out/peerphonic /peerphonic
VOLUME ["/data", "/music"]
EXPOSE 8080 42069/tcp 42069/udp
USER peerphonic
ENTRYPOINT ["/peerphonic"]
CMD ["serve", "--music", "/music", "--database", "/data/peerphonic.db", "--cache", "/data/cache", "--torrents", "/data/torrents"]
