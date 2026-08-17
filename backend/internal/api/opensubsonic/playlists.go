package opensubsonic

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
)

func (h *Handler) getPlaylists(writer http.ResponseWriter, request *http.Request) {
	owner := request.Form.Get("username")
	if owner == "" {
		owner = h.username
	}
	items, err := h.playlists.List(request.Context(), owner)
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read playlists")
		return
	}
	payload := &playlists{Items: make([]playlist, 0, len(items))}
	for _, item := range items {
		payload.Items = append(payload.Items, makePlaylist(item, false))
	}
	h.write(writer, request, http.StatusOK, response{Playlists: payload})
}

func (h *Handler) getPlaylist(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	item, err := h.playlists.Get(request.Context(), h.username, id)
	if err != nil {
		h.writePlaylistError(writer, request, err)
		return
	}
	payload := makePlaylist(item, true)
	h.write(writer, request, http.StatusOK, response{Playlist: &payload})
}

func (h *Handler) createPlaylist(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("playlistId")
	name := request.Form.Get("name")
	if id == "" && strings.TrimSpace(name) == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter name is missing")
		return
	}
	item, err := h.playlists.CreateOrReplace(
		request.Context(), h.username, id, name, request.Form["songId"],
	)
	if err != nil {
		h.writePlaylistError(writer, request, err)
		return
	}
	payload := makePlaylist(item, true)
	h.write(writer, request, http.StatusOK, response{Playlist: &payload})
}

func (h *Handler) updatePlaylist(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("playlistId")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter playlistId is missing")
		return
	}
	update := services.PlaylistUpdate{SongIDsToAdd: request.Form["songIdToAdd"]}
	if request.Form.Has("name") {
		value := request.Form.Get("name")
		update.Name = &value
	}
	if request.Form.Has("comment") {
		value := request.Form.Get("comment")
		update.Comment = &value
	}
	if request.Form.Has("public") {
		value, err := strconv.ParseBool(request.Form.Get("public"))
		if err != nil {
			h.writeError(writer, request, http.StatusBadRequest, 10, "Parameter public must be true or false")
			return
		}
		update.Public = &value
	}
	var err error
	update.SongIndexesToRemove, err = playlistIndexes(request.Form["songIndexToRemove"])
	if err != nil {
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
		return
	}
	if _, err := h.playlists.Update(request.Context(), h.username, id, update); err != nil {
		h.writePlaylistError(writer, request, err)
		return
	}
	h.write(writer, request, http.StatusOK, response{})
}

func (h *Handler) deletePlaylist(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	if err := h.playlists.Delete(request.Context(), h.username, id); err != nil {
		h.writePlaylistError(writer, request, err)
		return
	}
	h.write(writer, request, http.StatusOK, response{})
}

func (h *Handler) writePlaylistError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		h.writeError(writer, request, http.StatusNotFound, 70, "Playlist or song not found")
	case errors.Is(err, services.ErrInvalidPlaylist):
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
	default:
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to update playlist")
	}
}

func playlistIndexes(values []string) ([]int, error) {
	indexes := make([]int, 0, len(values))
	for _, value := range values {
		index, err := strconv.Atoi(value)
		if err != nil || index < 0 {
			return nil, errors.New("parameter songIndexToRemove must be a non-negative integer")
		}
		indexes = append(indexes, index)
	}
	return indexes, nil
}

func makePlaylist(item domain.Playlist, entries bool) playlist {
	result := playlist{
		ID: item.ID, Name: item.Name, Comment: item.Comment, Owner: item.Owner,
		Public: item.Public, Created: item.Created.UTC().Format(time.RFC3339),
		Changed: item.Changed.UTC().Format(time.RFC3339), SongCount: item.SongCount,
		Duration: int(item.Duration.Seconds()),
	}
	if entries {
		result.Entries = make([]child, 0, len(item.Tracks))
		for _, track := range item.Tracks {
			result.Entries = append(result.Entries, trackChild(track))
		}
	}
	return result
}
