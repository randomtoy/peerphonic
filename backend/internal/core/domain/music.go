package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"
)

// SourceRef identifies media without exposing provider-specific details to callers.
type SourceRef struct {
	Provider string
	Key      string
}

type Track struct {
	ID            string
	Title         string
	Artist        string
	ArtistID      string
	Album         string
	AlbumID       string
	AlbumArtist   string
	AlbumArtistID string
	Source        SourceRef
	TrackNumber   int
	DiscNumber    int
	Year          int
	Duration      time.Duration
	Size          int64
	BitRate       int
	Suffix        string
	ContentType   string
	CoverArtID    string
}

type Artist struct {
	ID         string
	Name       string
	AlbumCount int
}

type Album struct {
	ID         string
	Name       string
	Artist     string
	ArtistID   string
	Year       int
	SongCount  int
	Duration   time.Duration
	CoverArtID string
}

type SearchQuery struct {
	Text  string
	Limit int
}

type TrackSource struct {
	Ref      SourceRef
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
}

// StableID creates an opaque, deterministic identifier for a domain entity.
func StableID(kind string, parts ...string) string {
	values := append([]string{kind}, parts...)
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return kind + "_" + base64.RawURLEncoding.EncodeToString(sum[:12])
}
