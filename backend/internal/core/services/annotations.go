package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

var ErrInvalidAnnotation = errors.New("invalid media annotation")

type annotationCatalog interface {
	Track(ctx context.Context, id string) (domain.Track, error)
	TracksByAlbum(ctx context.Context, albumID string) ([]domain.Track, error)
	Artist(ctx context.Context, id string) (domain.Artist, error)
}

type AnnotationService struct {
	catalog annotationCatalog
	store   ports.MediaAnnotationStore
	now     func() time.Time
}

func NewAnnotationService(catalog annotationCatalog, store ports.MediaAnnotationStore) *AnnotationService {
	return &AnnotationService{catalog: catalog, store: store, now: time.Now}
}

func (s *AnnotationService) All(ctx context.Context, owner string) (map[domain.MediaRef]domain.MediaAnnotation, error) {
	items, err := s.store.MediaAnnotations(ctx, owner)
	if err != nil {
		return nil, err
	}
	return annotationMap(items), nil
}

func (s *AnnotationService) SetStarred(
	ctx context.Context,
	owner string,
	refs []domain.MediaRef,
	starred bool,
) error {
	resolved, err := s.resolveRefs(ctx, refs)
	if err != nil {
		return err
	}
	changed := make([]ports.MediaAnnotationUpdate, 0, len(resolved))
	for _, ref := range resolved {
		starredAt := time.Time{}
		if starred {
			starredAt = s.now().UTC()
		}
		changed = append(changed, ports.MediaAnnotationUpdate{
			Owner: owner, Media: ref, StarredAt: &starredAt,
		})
	}
	return s.store.UpdateMediaAnnotations(ctx, changed)
}

func (s *AnnotationService) SetRating(ctx context.Context, owner string, ref domain.MediaRef, rating int) error {
	if rating < 0 || rating > 5 {
		return fmt.Errorf("rating must be between 0 and 5: %w", ErrInvalidAnnotation)
	}
	resolved, err := s.resolveRef(ctx, ref)
	if err != nil {
		return err
	}
	return s.store.UpdateMediaAnnotations(ctx, []ports.MediaAnnotationUpdate{{
		Owner: owner, Media: resolved, Rating: &rating,
	}})
}

func (s *AnnotationService) Scrobble(
	ctx context.Context,
	owner string,
	ids []string,
	playedAt []time.Time,
	submission bool,
) error {
	if len(ids) == 0 {
		return fmt.Errorf("at least one song id is required: %w", ErrInvalidAnnotation)
	}
	if len(playedAt) != 0 && len(playedAt) != len(ids) {
		return fmt.Errorf("time parameter count must match id parameter count: %w", ErrInvalidAnnotation)
	}
	refs := make([]domain.MediaRef, len(ids))
	for index, id := range ids {
		refs[index] = domain.MediaRef{Type: domain.MediaSong, ID: id}
	}
	resolved, err := s.resolveRefs(ctx, refs)
	if err != nil || !submission {
		return err
	}
	updates := make([]ports.MediaAnnotationUpdate, 0, len(resolved))
	for index, ref := range resolved {
		lastPlayed := s.now().UTC()
		if len(playedAt) != 0 {
			lastPlayed = playedAt[index].UTC()
		}
		updates = append(updates, ports.MediaAnnotationUpdate{
			Owner: owner, Media: ref, PlayCountDelta: 1, LastPlayed: &lastPlayed,
		})
	}
	return s.store.UpdateMediaAnnotations(ctx, updates)
}

func (s *AnnotationService) Starred(ctx context.Context, owner string) (domain.StarredLibrary, error) {
	items, err := s.store.MediaAnnotations(ctx, owner)
	if err != nil {
		return domain.StarredLibrary{}, err
	}
	result := domain.StarredLibrary{Annotations: annotationMap(items)}
	for _, annotation := range items {
		if annotation.StarredAt.IsZero() {
			continue
		}
		switch annotation.Media.Type {
		case domain.MediaSong:
			track, err := s.catalog.Track(ctx, annotation.Media.ID)
			if errors.Is(err, ports.ErrNotFound) {
				continue
			}
			if err != nil {
				return domain.StarredLibrary{}, err
			}
			result.Tracks = append(result.Tracks, track)
		case domain.MediaAlbum:
			tracks, err := s.catalog.TracksByAlbum(ctx, annotation.Media.ID)
			if err != nil {
				return domain.StarredLibrary{}, err
			}
			if len(tracks) != 0 {
				result.Albums = append(result.Albums, annotationAlbum(annotation.Media.ID, tracks))
			}
		case domain.MediaArtist:
			artist, err := s.catalog.Artist(ctx, annotation.Media.ID)
			if errors.Is(err, ports.ErrNotFound) {
				continue
			}
			if err != nil {
				return domain.StarredLibrary{}, err
			}
			result.Artists = append(result.Artists, artist)
		}
	}
	return result, nil
}

func (s *AnnotationService) resolveRefs(ctx context.Context, refs []domain.MediaRef) ([]domain.MediaRef, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("at least one media id is required: %w", ErrInvalidAnnotation)
	}
	resolved := make([]domain.MediaRef, len(refs))
	for index, ref := range refs {
		var err error
		resolved[index], err = s.resolveRef(ctx, ref)
		if err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func (s *AnnotationService) resolveRef(ctx context.Context, ref domain.MediaRef) (domain.MediaRef, error) {
	ref.ID = strings.TrimSpace(ref.ID)
	if ref.ID == "" {
		return domain.MediaRef{}, fmt.Errorf("media id is empty: %w", ErrInvalidAnnotation)
	}
	switch ref.Type {
	case domain.MediaSong:
		if _, err := s.catalog.Track(ctx, ref.ID); err != nil {
			return domain.MediaRef{}, err
		}
	case domain.MediaAlbum:
		tracks, err := s.catalog.TracksByAlbum(ctx, ref.ID)
		if err != nil {
			return domain.MediaRef{}, err
		}
		if len(tracks) == 0 {
			return domain.MediaRef{}, ports.ErrNotFound
		}
	case domain.MediaArtist:
		if _, err := s.catalog.Artist(ctx, ref.ID); err != nil {
			return domain.MediaRef{}, err
		}
	case "":
		if _, err := s.catalog.Track(ctx, ref.ID); err == nil {
			ref.Type = domain.MediaSong
			break
		} else if !errors.Is(err, ports.ErrNotFound) {
			return domain.MediaRef{}, err
		}
		tracks, err := s.catalog.TracksByAlbum(ctx, ref.ID)
		if err != nil {
			return domain.MediaRef{}, err
		}
		if len(tracks) != 0 {
			ref.Type = domain.MediaAlbum
			break
		}
		if _, err := s.catalog.Artist(ctx, ref.ID); err != nil {
			return domain.MediaRef{}, err
		}
		ref.Type = domain.MediaArtist
	default:
		return domain.MediaRef{}, fmt.Errorf("unsupported media type %q: %w", ref.Type, ErrInvalidAnnotation)
	}
	return ref, nil
}

func annotationMap(items []domain.MediaAnnotation) map[domain.MediaRef]domain.MediaAnnotation {
	result := make(map[domain.MediaRef]domain.MediaAnnotation, len(items))
	for _, item := range items {
		result[item.Media] = item
	}
	return result
}

func annotationAlbum(id string, tracks []domain.Track) domain.Album {
	first := tracks[0]
	album := domain.Album{
		ID: id, Name: first.Album, Artist: first.AlbumArtist, ArtistID: first.AlbumArtistID,
		Year: first.Year, Genre: first.Genre, SongCount: len(tracks), CoverArtID: first.CoverArtID,
	}
	if album.Artist == "" {
		album.Artist = first.Artist
	}
	if album.ArtistID == "" {
		album.ArtistID = first.ArtistID
	}
	for _, track := range tracks {
		album.Duration += track.Duration
	}
	return album
}
