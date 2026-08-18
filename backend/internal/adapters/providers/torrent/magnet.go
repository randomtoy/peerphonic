package torrent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
)

// FetchMagnetMetadata resolves a magnet into a portable .torrent payload. It
// requests only the metadata; media files remain unselected until playback.
func (p *Provider) FetchMagnetMetadata(ctx context.Context, uri string) ([]byte, error) {
	magnet, err := metainfo.ParseMagnetUri(strings.TrimSpace(uri))
	if err != nil {
		return nil, fmt.Errorf("parse magnet URI: %w", err)
	}
	infoHash := magnet.InfoHash.HexString()
	metadataPath := filepath.Join(p.metadataRoot, infoHash+".torrent")
	if contents, err := os.ReadFile(metadataPath); err == nil {
		return contents, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read existing torrent metadata: %w", err)
	}

	p.operation.RLock()
	defer p.operation.RUnlock()
	client, err := p.ensureClient()
	if err != nil {
		return nil, err
	}
	torrent, err := client.AddMagnet(uri)
	if err != nil {
		return nil, fmt.Errorf("add magnet: %w", err)
	}
	defer torrent.Drop()
	select {
	case <-torrent.GotInfo():
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	metadata := torrent.Metainfo()
	if metadata.HashInfoBytes().HexString() != infoHash {
		return nil, fmt.Errorf("fetched torrent metadata hash does not match magnet")
	}
	var contents bytes.Buffer
	if err := metadata.Write(&contents); err != nil {
		return nil, fmt.Errorf("encode fetched torrent metadata: %w", err)
	}
	return contents.Bytes(), nil
}
