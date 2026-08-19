package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
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
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlbumCount int    `json:"albumCount"`
}

type ArtistAlias struct {
	AliasID    string    `json:"aliasId"`
	AliasName  string    `json:"aliasName"`
	TargetID   string    `json:"targetId"`
	TargetName string    `json:"targetName"`
	CreatedAt  time.Time `json:"createdAt"`
}

type TrackMetadataPatch struct {
	Title       *string
	Artist      *string
	Album       *string
	AlbumArtist *string
	Genre       *string
	Year        *int
	TrackNumber *int
	DiscNumber  *int
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

// CatalogNameKey normalizes a display name for provider-independent catalog identity.
// Display spelling remains untouched; only opaque artist and album IDs use this key.
func CatalogNameKey(name string) string {
	return cases.Fold().String(norm.NFKC.String(strings.Join(strings.Fields(name), " ")))
}

// CanonicalArtistID identifies an artist independently of the source provider.
func CanonicalArtistID(name string) string {
	return StableID("artist", CatalogNameKey(name))
}

// CanonicalAlbumID identifies an album by its album artist and name independently
// of the source provider.
func CanonicalAlbumID(albumArtist, album string) string {
	return StableID("album", CanonicalArtistID(albumArtist), CatalogNameKey(album))
}

// CanonicalTrackID identifies one logical recording in an album. Physical
// providers and peers remain separate source references for this identity.
func CanonicalTrackID(track Track) string {
	albumArtist := strings.TrimSpace(track.AlbumArtist)
	if albumArtist == "" {
		albumArtist = track.Artist
	}
	title, inferredNumber := canonicalTrackTitle(track.Title)
	trackNumber := track.TrackNumber
	if trackNumber == 0 {
		trackNumber = inferredNumber
	}
	return StableID(
		"track",
		CanonicalAlbumID(albumArtist, track.Album),
		CanonicalArtistID(track.Artist),
		strconv.Itoa(track.DiscNumber),
		strconv.Itoa(trackNumber),
		title,
	)
}

// HasCanonicalTrackIdentity reports whether the fields needed for safe
// provider-independent matching are available.
func HasCanonicalTrackIdentity(track Track) bool {
	return strings.TrimSpace(track.Title) != "" && strings.TrimSpace(track.Artist) != "" &&
		strings.TrimSpace(track.Album) != ""
}

func canonicalTrackTitle(value string) (string, int) {
	key := CatalogNameKey(value)
	runes := []rune(key)
	index := 0
	for index < len(runes) && unicode.IsDigit(runes[index]) && index < 3 {
		index++
	}
	if index == 0 || index == len(runes) || unicode.IsLetter(runes[index]) || unicode.IsDigit(runes[index]) {
		return key, 0
	}
	number, err := strconv.Atoi(string(runes[:index]))
	if err != nil {
		return key, 0
	}
	for index < len(runes) && !unicode.IsLetter(runes[index]) && !unicode.IsDigit(runes[index]) {
		index++
	}
	if index == len(runes) {
		return key, 0
	}
	return strings.TrimSpace(string(runes[index:])), number
}
