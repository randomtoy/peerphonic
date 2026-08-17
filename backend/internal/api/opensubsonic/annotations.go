package opensubsonic

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
)

func (h *Handler) setStarred(writer http.ResponseWriter, request *http.Request, starred bool) {
	if h.annotations == nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Media annotations are not configured")
		return
	}
	refs := annotationRefs(request)
	if err := h.annotations.SetStarred(request.Context(), h.username, refs, starred); err != nil {
		h.writeAnnotationError(writer, request, err)
		return
	}
	h.write(writer, request, http.StatusOK, response{})
}

func (h *Handler) setRating(writer http.ResponseWriter, request *http.Request) {
	if h.annotations == nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Media annotations are not configured")
		return
	}
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	rating, err := strconv.Atoi(request.Form.Get("rating"))
	if err != nil || rating < 0 || rating > 5 {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Parameter rating must be between 0 and 5")
		return
	}
	if err := h.annotations.SetRating(
		request.Context(), h.username, domain.MediaRef{ID: id}, rating,
	); err != nil {
		h.writeAnnotationError(writer, request, err)
		return
	}
	h.write(writer, request, http.StatusOK, response{})
}

func (h *Handler) scrobble(writer http.ResponseWriter, request *http.Request) {
	if h.annotations == nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Media annotations are not configured")
		return
	}
	ids := request.Form["id"]
	times, err := scrobbleTimes(request.Form["time"])
	if err != nil {
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
		return
	}
	submission := true
	if request.Form.Has("submission") {
		submission, err = strconv.ParseBool(request.Form.Get("submission"))
		if err != nil {
			h.writeError(writer, request, http.StatusBadRequest, 10, "Parameter submission must be true or false")
			return
		}
	}
	if err := h.annotations.Scrobble(request.Context(), h.username, ids, times, submission); err != nil {
		h.writeAnnotationError(writer, request, err)
		return
	}
	h.write(writer, request, http.StatusOK, response{})
}

func (h *Handler) getStarred(writer http.ResponseWriter, request *http.Request, endpoint string) {
	responsePayload := response{Starred2: &starredLibrary{
		Artists: []artistID3{}, Albums: []albumID3{}, Songs: []child{},
	}}
	if endpoint == "getStarred" {
		responsePayload.Starred = &starred{Artists: []artist{}, Albums: []child{}, Songs: []child{}}
		responsePayload.Starred2 = nil
	}
	if request.Form.Has("musicFolderId") && request.Form.Get("musicFolderId") != musicFolderID {
		h.write(writer, request, http.StatusOK, responsePayload)
		return
	}
	if h.annotations == nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Media annotations are not configured")
		return
	}
	items, err := h.annotations.Starred(request.Context(), h.username)
	if err != nil {
		h.writeAnnotationError(writer, request, err)
		return
	}
	for _, item := range items.Artists {
		if responsePayload.Starred != nil {
			responsePayload.Starred.Artists = append(responsePayload.Starred.Artists, artist{
				ID: item.ID, Name: item.Name,
			})
		} else {
			responsePayload.Starred2.Artists = append(responsePayload.Starred2.Artists, artistID3{
				ID: item.ID, Name: item.Name, AlbumCount: item.AlbumCount,
			})
		}
	}
	for _, item := range items.Albums {
		if responsePayload.Starred != nil {
			responsePayload.Starred.Albums = append(responsePayload.Starred.Albums, albumChild(item))
		} else {
			responsePayload.Starred2.Albums = append(responsePayload.Starred2.Albums, makeAlbumID3(item))
		}
	}
	for _, item := range items.Tracks {
		if responsePayload.Starred != nil {
			responsePayload.Starred.Songs = append(responsePayload.Starred.Songs, trackChild(item))
		} else {
			responsePayload.Starred2.Songs = append(responsePayload.Starred2.Songs, trackChild(item))
		}
	}
	h.write(writer, request, http.StatusOK, responsePayload)
}

func annotationRefs(request *http.Request) []domain.MediaRef {
	refs := make([]domain.MediaRef, 0,
		len(request.Form["id"])+len(request.Form["albumId"])+len(request.Form["artistId"]))
	for _, id := range request.Form["id"] {
		refs = append(refs, domain.MediaRef{ID: id})
	}
	for _, id := range request.Form["albumId"] {
		refs = append(refs, domain.MediaRef{Type: domain.MediaAlbum, ID: id})
	}
	for _, id := range request.Form["artistId"] {
		refs = append(refs, domain.MediaRef{Type: domain.MediaArtist, ID: id})
	}
	return refs
}

func scrobbleTimes(values []string) ([]time.Time, error) {
	times := make([]time.Time, len(values))
	for index, value := range values {
		milliseconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, errors.New("parameter time must be milliseconds since the Unix epoch")
		}
		times[index] = time.UnixMilli(milliseconds).UTC()
	}
	return times, nil
}

func (h *Handler) writeAnnotationError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		h.writeError(writer, request, http.StatusNotFound, 70, "Media item not found")
	case errors.Is(err, services.ErrInvalidAnnotation):
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
	default:
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to update media annotation")
	}
}

func (h *Handler) decorateAnnotations(ctx context.Context, payload *response) {
	if h.annotations == nil || payload.Error != nil || !payloadContainsMedia(payload) {
		return
	}
	annotations, err := h.annotations.All(ctx, h.username)
	if err != nil {
		return
	}
	if payload.Indexes != nil {
		for index := range payload.Indexes.Indexes {
			for item := range payload.Indexes.Indexes[index].Artists {
				applyArtistAnnotation(&payload.Indexes.Indexes[index].Artists[item], annotations)
			}
		}
	}
	if payload.Directory != nil {
		for index := range payload.Directory.Children {
			applyChildAnnotation(&payload.Directory.Children[index], annotations)
		}
	}
	if payload.Artists != nil {
		for index := range payload.Artists.Indexes {
			for item := range payload.Artists.Indexes[index].Artists {
				applyArtistID3Annotation(&payload.Artists.Indexes[index].Artists[item], annotations)
			}
		}
	}
	if payload.ArtistDetail != nil {
		applyArtistID3Annotation(payload.ArtistDetail, annotations)
		for index := range payload.ArtistDetail.Albums {
			applyAlbumAnnotation(&payload.ArtistDetail.Albums[index], annotations)
		}
	}
	if payload.Album != nil {
		applyAlbumAnnotation(payload.Album, annotations)
		for index := range payload.Album.Songs {
			applyChildAnnotation(&payload.Album.Songs[index], annotations)
		}
	}
	if payload.AlbumList2 != nil {
		for index := range payload.AlbumList2.Albums {
			applyAlbumAnnotation(&payload.AlbumList2.Albums[index], annotations)
		}
	}
	decorateSongs(payload.SongsByGenre, annotations)
	decorateSongs(payload.RandomSongs, annotations)
	if payload.Song != nil {
		applyChildAnnotation(payload.Song, annotations)
	}
	if payload.SearchResult3 != nil {
		for index := range payload.SearchResult3.Artists {
			applyArtistID3Annotation(&payload.SearchResult3.Artists[index], annotations)
		}
		for index := range payload.SearchResult3.Albums {
			applyAlbumAnnotation(&payload.SearchResult3.Albums[index], annotations)
		}
		for index := range payload.SearchResult3.Songs {
			applyChildAnnotation(&payload.SearchResult3.Songs[index], annotations)
		}
	}
	if payload.Playlist != nil {
		for index := range payload.Playlist.Entries {
			applyChildAnnotation(&payload.Playlist.Entries[index], annotations)
		}
	}
	if payload.PlayQueue != nil {
		for index := range payload.PlayQueue.Entries {
			applyChildAnnotation(&payload.PlayQueue.Entries[index], annotations)
		}
	}
	decorateStarred(payload.Starred, annotations)
	decorateStarred2(payload.Starred2, annotations)
}

func payloadContainsMedia(payload *response) bool {
	return payload.Indexes != nil || payload.Directory != nil || payload.Artists != nil ||
		payload.ArtistDetail != nil || payload.Album != nil || payload.AlbumList2 != nil ||
		payload.SongsByGenre != nil || payload.RandomSongs != nil || payload.Song != nil ||
		payload.SearchResult3 != nil || payload.Playlist != nil ||
		payload.Starred != nil || payload.Starred2 != nil || payload.PlayQueue != nil
}

func decorateSongs(items *songs, annotations map[domain.MediaRef]domain.MediaAnnotation) {
	if items == nil {
		return
	}
	for index := range items.Songs {
		applyChildAnnotation(&items.Songs[index], annotations)
	}
}

func decorateStarred(items *starred, annotations map[domain.MediaRef]domain.MediaAnnotation) {
	if items == nil {
		return
	}
	for index := range items.Artists {
		applyArtistAnnotation(&items.Artists[index], annotations)
	}
	for index := range items.Albums {
		applyChildAnnotation(&items.Albums[index], annotations)
	}
	for index := range items.Songs {
		applyChildAnnotation(&items.Songs[index], annotations)
	}
}

func decorateStarred2(items *starredLibrary, annotations map[domain.MediaRef]domain.MediaAnnotation) {
	if items == nil {
		return
	}
	for index := range items.Artists {
		applyArtistID3Annotation(&items.Artists[index], annotations)
	}
	for index := range items.Albums {
		applyAlbumAnnotation(&items.Albums[index], annotations)
	}
	for index := range items.Songs {
		applyChildAnnotation(&items.Songs[index], annotations)
	}
}

func applyChildAnnotation(item *child, annotations map[domain.MediaRef]domain.MediaAnnotation) {
	mediaType := domain.MediaSong
	if item.IsDir {
		mediaType = domain.MediaAlbum
		if strings.HasPrefix(item.ID, "artist_") {
			mediaType = domain.MediaArtist
		}
	}
	annotation := annotations[domain.MediaRef{Type: mediaType, ID: item.ID}]
	item.Starred, item.UserRating, item.PlayCount, item.Played = annotationValues(annotation)
}

func applyAlbumAnnotation(item *albumID3, annotations map[domain.MediaRef]domain.MediaAnnotation) {
	annotation := annotations[domain.MediaRef{Type: domain.MediaAlbum, ID: item.ID}]
	item.Starred, item.UserRating, item.PlayCount, item.Played = annotationValues(annotation)
}

func applyArtistID3Annotation(item *artistID3, annotations map[domain.MediaRef]domain.MediaAnnotation) {
	annotation := annotations[domain.MediaRef{Type: domain.MediaArtist, ID: item.ID}]
	item.Starred, item.UserRating, item.PlayCount, _ = annotationValues(annotation)
}

func applyArtistAnnotation(item *artist, annotations map[domain.MediaRef]domain.MediaAnnotation) {
	annotation := annotations[domain.MediaRef{Type: domain.MediaArtist, ID: item.ID}]
	item.Starred, item.UserRating, item.PlayCount, _ = annotationValues(annotation)
}

func annotationValues(annotation domain.MediaAnnotation) (string, int, int64, string) {
	var starred, played string
	if !annotation.StarredAt.IsZero() {
		starred = annotation.StarredAt.UTC().Format(time.RFC3339Nano)
	}
	if !annotation.LastPlayed.IsZero() {
		played = annotation.LastPlayed.UTC().Format(time.RFC3339Nano)
	}
	return starred, annotation.Rating, annotation.PlayCount, played
}
