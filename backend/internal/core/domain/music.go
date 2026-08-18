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
	TrackNumber   int
	DiscNumber    int
	Year          int
	Genre         string
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
	Genre      string
	SongCount  int
	Duration   time.Duration
	CoverArtID string
}

type SearchQuery struct {
	Text  string
	Limit int
}

type Genre struct {
	Name       string
	SongCount  int
	AlbumCount int
}

type Playlist struct {
	ID        string
	Name      string
	Comment   string
	Owner     string
	Public    bool
	Created   time.Time
	Changed   time.Time
	SongCount int
	Duration  time.Duration
	Tracks    []Track
}

type MediaType string

const (
	MediaSong   MediaType = "song"
	MediaAlbum  MediaType = "album"
	MediaArtist MediaType = "artist"
)

type MediaRef struct {
	Type MediaType
	ID   string
}

type MediaAnnotation struct {
	Owner      string
	Media      MediaRef
	StarredAt  time.Time
	Rating     int
	PlayCount  int64
	LastPlayed time.Time
}

type StarredLibrary struct {
	Artists     []Artist
	Albums      []Album
	Tracks      []Track
	Annotations map[MediaRef]MediaAnnotation
}

type PlayQueue struct {
	Owner      string
	CurrentID  string
	PositionMS int64
	Changed    time.Time
	ChangedBy  string
	Tracks     []Track
}

type TrackSource struct {
	Track        Track
	Ref          SourceRef
	DisplayPath  string
	Availability SourceAvailability
	DiscoveredAt time.Time
}

// SourceCollection is a provider-neutral group of tracks discovered next to
// a selected source. Providers may map it to a remote directory, release, or
// another native grouping without exposing that detail to application code.
type SourceCollection struct {
	Name       string
	Artist     string
	CoverArtID string
	Tracks     []TrackSource
}

// SourceAvailability describes provider-neutral hints for choosing between
// remote copies of the same track.
type SourceAvailability struct {
	Peer             string
	UploadSpeed      int64
	QueueLength      int64
	FreeUploadSlot   bool
	RequiresApproval bool
}

// StableID creates an opaque, deterministic identifier for a domain entity.
func StableID(kind string, parts ...string) string {
	values := append([]string{kind}, parts...)
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return kind + "_" + base64.RawURLEncoding.EncodeToString(sum[:12])
}
