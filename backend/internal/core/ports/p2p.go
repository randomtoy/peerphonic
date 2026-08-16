package ports

import (
	"context"
	"io"
)

type Peer struct {
	ID   string
	Name string
}

// P2PTransport owns discovery and byte transfer only. Music discovery belongs
// in a SourceProvider adapter and must not leak transport concepts into core.
type P2PTransport interface {
	Discover(ctx context.Context) ([]Peer, error)
	Fetch(ctx context.Context, peerID, resource string) (io.ReadCloser, error)
}
