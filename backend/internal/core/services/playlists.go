package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

var ErrInvalidPlaylist = errors.New("invalid playlist")

type playlistCatalog interface {
	Track(ctx context.Context, id string) (domain.Track, error)
	Playlists(ctx context.Context, owner string) ([]domain.Playlist, error)
	Playlist(ctx context.Context, id string) (domain.Playlist, error)
	SavePlaylist(ctx context.Context, playlist domain.Playlist) error
	DeletePlaylist(ctx context.Context, id string) error
}

type PlaylistUpdate struct {
	Name                *string
	Comment             *string
	Public              *bool
	SongIDsToAdd        []string
	SongIndexesToRemove []int
}

type PlaylistService struct {
	catalog playlistCatalog
	now     func() time.Time
	newID   func() (string, error)
}

func NewPlaylistService(catalog playlistCatalog) *PlaylistService {
	return &PlaylistService{catalog: catalog, now: time.Now, newID: newPlaylistID}
}

func (s *PlaylistService) List(ctx context.Context, owner string) ([]domain.Playlist, error) {
	return s.catalog.Playlists(ctx, owner)
}

func (s *PlaylistService) Get(ctx context.Context, owner, id string) (domain.Playlist, error) {
	playlist, err := s.catalog.Playlist(ctx, id)
	if err != nil {
		return domain.Playlist{}, err
	}
	if playlist.Owner != owner && !playlist.Public {
		return domain.Playlist{}, ports.ErrNotFound
	}
	return playlist, nil
}

func (s *PlaylistService) CreateOrReplace(
	ctx context.Context,
	owner, id, name string,
	songIDs []string,
) (domain.Playlist, error) {
	name = strings.TrimSpace(name)
	now := s.now().UTC()
	var playlist domain.Playlist
	if id == "" {
		if name == "" {
			return domain.Playlist{}, fmt.Errorf("playlist name is required: %w", ErrInvalidPlaylist)
		}
		var err error
		id, err = s.newID()
		if err != nil {
			return domain.Playlist{}, fmt.Errorf("create playlist id: %w", err)
		}
		playlist = domain.Playlist{ID: id, Name: name, Owner: owner, Created: now}
	} else {
		var err error
		playlist, err = s.owned(ctx, owner, id)
		if err != nil {
			return domain.Playlist{}, err
		}
		if name != "" {
			playlist.Name = name
		}
	}
	tracks, err := s.resolveTracks(ctx, songIDs)
	if err != nil {
		return domain.Playlist{}, err
	}
	playlist.Tracks = tracks
	playlist.Changed = now
	if err := s.catalog.SavePlaylist(ctx, playlist); err != nil {
		return domain.Playlist{}, err
	}
	return summarizePlaylist(playlist), nil
}

func (s *PlaylistService) Update(
	ctx context.Context,
	owner, id string,
	update PlaylistUpdate,
) (domain.Playlist, error) {
	playlist, err := s.owned(ctx, owner, id)
	if err != nil {
		return domain.Playlist{}, err
	}
	if update.Name != nil {
		name := strings.TrimSpace(*update.Name)
		if name == "" {
			return domain.Playlist{}, fmt.Errorf("playlist name cannot be empty: %w", ErrInvalidPlaylist)
		}
		playlist.Name = name
	}
	if update.Comment != nil {
		playlist.Comment = *update.Comment
	}
	if update.Public != nil {
		playlist.Public = *update.Public
	}
	remove := make(map[int]struct{}, len(update.SongIndexesToRemove))
	for _, index := range update.SongIndexesToRemove {
		if index < 0 || index >= len(playlist.Tracks) {
			return domain.Playlist{}, fmt.Errorf("song index %d is out of range: %w", index, ErrInvalidPlaylist)
		}
		remove[index] = struct{}{}
	}
	remaining := make([]domain.Track, 0, len(playlist.Tracks)-len(remove)+len(update.SongIDsToAdd))
	for index, track := range playlist.Tracks {
		if _, removed := remove[index]; !removed {
			remaining = append(remaining, track)
		}
	}
	added, err := s.resolveTracks(ctx, update.SongIDsToAdd)
	if err != nil {
		return domain.Playlist{}, err
	}
	playlist.Tracks = append(remaining, added...)
	playlist.Changed = s.now().UTC()
	if err := s.catalog.SavePlaylist(ctx, playlist); err != nil {
		return domain.Playlist{}, err
	}
	return summarizePlaylist(playlist), nil
}

func (s *PlaylistService) Delete(ctx context.Context, owner, id string) error {
	if _, err := s.owned(ctx, owner, id); err != nil {
		return err
	}
	return s.catalog.DeletePlaylist(ctx, id)
}

func (s *PlaylistService) owned(ctx context.Context, owner, id string) (domain.Playlist, error) {
	playlist, err := s.catalog.Playlist(ctx, id)
	if err != nil {
		return domain.Playlist{}, err
	}
	if playlist.Owner != owner {
		return domain.Playlist{}, ports.ErrNotFound
	}
	return playlist, nil
}

func (s *PlaylistService) resolveTracks(ctx context.Context, ids []string) ([]domain.Track, error) {
	tracks := make([]domain.Track, 0, len(ids))
	for _, id := range ids {
		track, err := s.catalog.Track(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("resolve playlist song %q: %w", id, err)
		}
		tracks = append(tracks, track)
	}
	return tracks, nil
}

func summarizePlaylist(playlist domain.Playlist) domain.Playlist {
	playlist.SongCount = len(playlist.Tracks)
	playlist.Duration = 0
	for _, track := range playlist.Tracks {
		playlist.Duration += track.Duration
	}
	return playlist
}

func newPlaylistID() (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "playlist_" + base64.RawURLEncoding.EncodeToString(random[:]), nil
}
